package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

// MaxQuantityPerLine bounds a single product line so a typo cannot overflow money math.
const MaxQuantityPerLine = 1000

type ItemInput struct {
	ProductID string
	Quantity  int
}

type CalculateInput struct {
	MemberCard string // optional, any casing/spacing
	Items      []ItemInput
	// ClientKey is an optional stable key for one customer (for example one
	// browser session). Whoever holds a limited product can price it again
	// under the same key; any other key is rejected. Without it, every order
	// is a different holder.
	ClientKey string
}

// CalculateOrderDeps are the ports CalculateOrder needs.
type CalculateOrderDeps struct {
	Products ProductRepository
	Members  MemberRepository
	Rules    RuleRepository
	Limits   LimitRepository
	Orders   OrderRepository
	Limiter  SlotLimiter
	Clock    Clock
	IDs      IDGenerator
}

type CalculateOrder struct {
	d CalculateOrderDeps
}

func NewCalculateOrder(d CalculateOrderDeps) *CalculateOrder {
	return &CalculateOrder{d: d}
}

// Execute validates the request, prices it, records any limited products as
// ordered, saves the order and returns the quote. Nothing is reserved or
// saved unless every validation step has passed.
func (u *CalculateOrder) Execute(ctx context.Context, in CalculateInput) (domain.Quote, error) {
	merged, err := mergeItems(in.Items)
	if err != nil {
		return domain.Quote{}, err
	}

	carts, err := u.resolveCarts(ctx, merged)
	if err != nil {
		return domain.Quote{}, err
	}

	card := domain.NormalizeCard(in.MemberCard)
	if card != "" {
		if err := u.checkMember(ctx, card); err != nil {
			return domain.Quote{}, err
		}
	}

	rules, err := u.d.Rules.ActiveRules(ctx)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("load rules: %w", err)
	}
	quote, err := domain.Calculate(carts, card != "", rules)
	if err != nil {
		return domain.Quote{}, err
	}

	orderID := u.d.IDs.NewID()
	holder := in.ClientKey
	if holder == "" {
		holder = orderID
	}
	claimed, err := u.claimLimited(ctx, carts, holder)
	if err != nil {
		return domain.Quote{}, err
	}

	order := domain.Order{
		ID:         orderID,
		MemberCard: card,
		Carts:      carts,
		Quote:      quote,
		CreatedAt:  u.d.Clock.Now(),
	}
	if err := u.d.Orders.Save(ctx, order); err != nil {
		// The order was not stored, so the customer must be able to try again.
		u.release(ctx, claimed, holder)
		return domain.Quote{}, fmt.Errorf("save order: %w", err)
	}
	return quote, nil
}

type mergedItem struct {
	id  domain.ProductID
	qty int
}

// mergeItems validates quantities and merges repeated products, keeping first-seen order.
func mergeItems(items []ItemInput) ([]mergedItem, error) {
	if len(items) == 0 {
		return nil, domain.ErrEmptyOrder
	}
	index := make(map[domain.ProductID]int, len(items))
	var out []mergedItem
	for _, it := range items {
		if it.Quantity < 1 || it.Quantity > MaxQuantityPerLine {
			return nil, fmt.Errorf("%w: %q has quantity %d", domain.ErrInvalidQuantity, it.ProductID, it.Quantity)
		}
		id := domain.ProductID(it.ProductID)
		if i, ok := index[id]; ok {
			out[i].qty += it.Quantity
			continue
		}
		index[id] = len(out)
		out = append(out, mergedItem{id: id, qty: it.Quantity})
	}
	for _, m := range out {
		if m.qty > MaxQuantityPerLine {
			return nil, fmt.Errorf("%w: %q has quantity %d", domain.ErrInvalidQuantity, m.id, m.qty)
		}
	}
	return out, nil
}

func (u *CalculateOrder) resolveCarts(ctx context.Context, merged []mergedItem) ([]domain.Cart, error) {
	catalog, err := u.d.Products.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	byID := make(map[domain.ProductID]domain.Product, len(catalog))
	for _, p := range catalog {
		byID[p.ID] = p
	}
	carts := make([]domain.Cart, 0, len(merged))
	for _, m := range merged {
		p, ok := byID[m.id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", domain.ErrUnknownProduct, m.id)
		}
		carts = append(carts, domain.Cart{Product: p, Quantity: m.qty})
	}
	return carts, nil
}

func (u *CalculateOrder) checkMember(ctx context.Context, card string) error {
	m, found, err := u.d.Members.FindByCard(ctx, card)
	if err != nil {
		return fmt.Errorf("find member: %w", err)
	}
	if !found || !m.Active {
		return domain.ErrInvalidMemberCard
	}
	return nil
}

// claimLimited records every ordered product that has an active limit as
// ordered by holder, in product-ID order so a multi-product order behaves the
// same way every time. It returns the products this call newly claimed (a
// claim holder already had is not included). If any product cannot be claimed,
// the ones claimed so far are released before the error is returned.
func (u *CalculateOrder) claimLimited(ctx context.Context, lines []domain.Cart, holder string) ([]domain.ProductID, error) {
	limits, err := u.d.Limits.ActiveLimits(ctx)
	if err != nil {
		return nil, fmt.Errorf("load limits: %w", err)
	}
	ordered := append([]domain.Cart(nil), lines...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Product.ID < ordered[j].Product.ID })

	var claimed []domain.ProductID
	for _, l := range ordered {
		window, limited := limits[l.Product.ID]
		if !limited {
			continue
		}
		fresh, err := u.d.Limiter.Claim(ctx, l.Product.ID, holder, window)
		switch {
		case err == nil:
			if fresh {
				claimed = append(claimed, l.Product.ID)
			}
		case errors.Is(err, domain.ErrSlotHeld):
			u.release(ctx, claimed, holder)
			return nil, &domain.SlotHeldError{ProductID: l.Product.ID, ProductName: l.Product.Name}
		default:
			u.release(ctx, claimed, holder)
			return nil, fmt.Errorf("claim %s: %w", l.Product.ID, err)
		}
	}
	return claimed, nil
}

// release gives back claims this request made. It must still work when the
// request was cancelled, and a failure here is only logged: the claim then
// simply expires at the end of its window.
func (u *CalculateOrder) release(ctx context.Context, products []domain.ProductID, holder string) {
	if len(products) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	for _, id := range products {
		if err := u.d.Limiter.Release(ctx, id, holder); err != nil {
			slog.Warn("could not release limited product after a failed order", "product", id, "err", err)
		}
	}
}
