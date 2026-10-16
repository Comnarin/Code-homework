package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"

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
}

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

type CalculateOrder struct{ d CalculateOrderDeps }

func NewCalculateOrder(d CalculateOrderDeps) *CalculateOrder { return &CalculateOrder{d: d} }

// Execute validates the request, prices it, records any limited products as
// ordered, saves the order and returns the quote. Nothing is reserved or
// saved unless every validation step has passed.
func (u *CalculateOrder) Execute(ctx context.Context, in CalculateInput) (domain.Quote, error) {
	merged, err := mergeItems(in.Items)
	if err != nil {
		return domain.Quote{}, err
	}

	lines, err := u.resolveLines(ctx, merged)
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
	quote, err := domain.Calculate(lines, card != "", rules)
	if err != nil {
		return domain.Quote{}, err
	}

	if err := u.claimLimited(ctx, lines); err != nil {
		return domain.Quote{}, err
	}

	order := domain.Order{
		ID:         u.d.IDs.NewID(),
		MemberCard: card,
		Lines:      lines,
		Quote:      quote,
		CreatedAt:  u.d.Clock.Now(),
	}
	if err := u.d.Orders.Save(ctx, order); err != nil {
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

func (u *CalculateOrder) resolveLines(ctx context.Context, merged []mergedItem) ([]domain.Line, error) {
	catalog, err := u.d.Products.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	byID := make(map[domain.ProductID]domain.Product, len(catalog))
	for _, p := range catalog {
		byID[p.ID] = p
	}
	lines := make([]domain.Line, 0, len(merged))
	for _, m := range merged {
		p, ok := byID[m.id]
		if !ok {
			return nil, fmt.Errorf("%w: %q", domain.ErrUnknownProduct, m.id)
		}
		lines = append(lines, domain.Line{Product: p, Quantity: m.qty})
	}
	return lines, nil
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
// ordered, in product-ID order so a multi-product order behaves the same way
// every time.
func (u *CalculateOrder) claimLimited(ctx context.Context, lines []domain.Line) error {
	limits, err := u.d.Limits.ActiveLimits(ctx)
	if err != nil {
		return fmt.Errorf("load limits: %w", err)
	}
	ordered := append([]domain.Line(nil), lines...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Product.ID < ordered[j].Product.ID })

	for _, l := range ordered {
		window, limited := limits[l.Product.ID]
		if !limited {
			continue
		}
		err := u.d.Limiter.Claim(ctx, l.Product.ID, window)
		switch {
		case err == nil:
		case errors.Is(err, domain.ErrSlotHeld):
			return &domain.SlotHeldError{ProductID: l.Product.ID, ProductName: l.Product.Name}
		default:
			return fmt.Errorf("claim %s: %w", l.Product.ID, err)
		}
	}
	return nil
}
