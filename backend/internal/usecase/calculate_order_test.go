package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

type fixture struct {
	products *fakeProducts
	members  *fakeMembers
	rules    *fakeRules
	limits   *fakeLimits
	orders   *fakeOrders
	limiter  *fakeLimiter
	uc       *CalculateOrder
}

var testNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pair, err := domain.NewBundlePercent("PAIR_5", 10, []domain.ProductID{"orange", "pink", "green"}, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	member, err := domain.NewMemberPercent("MEMBER_10", 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		products: &fakeProducts{items: seedProducts()},
		members: &fakeMembers{byCard: map[string]domain.Member{
			"MEM0001": {CardNumber: "MEM0001", FullName: "Active Member", Active: true},
			"MEM0002": {CardNumber: "MEM0002", FullName: "Suspended Member", Active: false},
		}},
		rules:   &fakeRules{rules: []domain.Rule{pair, member}},
		limits:  &fakeLimits{limits: map[domain.ProductID]time.Duration{"red": time.Hour}},
		orders:  &fakeOrders{},
		limiter: &fakeLimiter{},
	}
	f.uc = NewCalculateOrder(CalculateOrderDeps{
		Products: f.products, Members: f.members, Rules: f.rules, Limits: f.limits,
		Orders: f.orders, Limiter: f.limiter, Clock: fixedClock{testNow}, IDs: &seqIDs{},
	})
	return f
}

func items(pairs ...any) []ItemInput {
	var out []ItemInput
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, ItemInput{ProductID: pairs[i].(string), Quantity: pairs[i+1].(int)})
	}
	return out
}

func TestCalculateOrder_PricesAPairDiscountAndSavesTheOrder(t *testing.T) {
	f := newFixture(t)

	q, err := f.uc.Execute(context.Background(), CalculateInput{Items: items("orange", 2)})
	if err != nil {
		t.Fatal(err)
	}
	if q.TotalBefore != 24000 || q.FinalTotal != 22800 {
		t.Fatalf("got %+v", q)
	}
	if len(f.orders.saved) != 1 {
		t.Fatalf("want 1 saved order, got %d", len(f.orders.saved))
	}
	o := f.orders.saved[0]
	if o.ID != "order-1" || !o.CreatedAt.Equal(testNow) || o.MemberCard != "" {
		t.Fatalf("saved order wrong: %+v", o)
	}
	if len(o.Carts) != 1 || o.Carts[0].Product.ID != "orange" || o.Carts[0].Quantity != 2 {
		t.Fatalf("saved carts wrong: %+v", o.Carts)
	}
}

func TestCalculateOrder_AppliesMemberDiscountAfterPairDiscount(t *testing.T) {
	f := newFixture(t)

	// Card is entered messily on purpose: the use case normalizes it.
	q, err := f.uc.Execute(context.Background(), CalculateInput{
		MemberCard: "  mem0001 ", Items: items("orange", 2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.MemberDiscount() != 2280 || q.FinalTotal != 20520 {
		t.Fatalf("got member=%d final=%d, want 2280/20520", q.MemberDiscount(), q.FinalTotal)
	}
	if got := f.orders.saved[0].MemberCard; got != "MEM0001" {
		t.Fatalf("saved card = %q, want MEM0001", got)
	}
}

func TestCalculateOrder_MergesDuplicateProductLines(t *testing.T) {
	f := newFixture(t)

	q, err := f.uc.Execute(context.Background(), CalculateInput{
		Items: items("orange", 1, "orange", 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.FinalTotal != 22800 { // one pair discount, as if orange x2
		t.Fatalf("FinalTotal = %d, want 22800", q.FinalTotal)
	}
	if len(f.orders.saved[0].Carts) != 1 {
		t.Fatalf("duplicate carts should merge, got %+v", f.orders.saved[0].Carts)
	}
}

func TestCalculateOrder_RedOrderedWithinTheHourBlocksTheNextRedOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.uc.Execute(ctx, CalculateInput{Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}

	// Anyone asking for Red in the same window is rejected, even with a valid card.
	_, err := f.uc.Execute(ctx, CalculateInput{MemberCard: "MEM0001", Items: items("red", 1)})

	var held *domain.SlotHeldError
	if !errors.As(err, &held) || held.ProductName != "Red set" || !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want SlotHeldError for Red set", err)
	}
	if len(f.orders.saved) != 1 {
		t.Fatalf("the rejected order must not be saved, saved = %d", len(f.orders.saved))
	}
}

func TestCalculateOrder_OrdersWithoutRedStillWorkWhileRedIsHeld(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.uc.Execute(ctx, CalculateInput{Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}

	q, err := f.uc.Execute(ctx, CalculateInput{Items: items("blue", 2)})
	if err != nil {
		t.Fatalf("an order without Red must not be affected: %v", err)
	}
	if q.FinalTotal != 6000 {
		t.Fatalf("FinalTotal = %d, want 6000", q.FinalTotal)
	}
}

func TestCalculateOrder_OnlyClaimsSlotsForLimitedProducts(t *testing.T) {
	f := newFixture(t)

	if _, err := f.uc.Execute(context.Background(), CalculateInput{Items: items("blue", 1, "red", 1)}); err != nil {
		t.Fatal(err)
	}
	if len(f.limiter.claims) != 1 || f.limiter.claims[0] != "red" {
		t.Fatalf("claims = %v, want only [red]", f.limiter.claims)
	}
}

func TestCalculateOrder_RejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name  string
		items []ItemInput
		want  error
	}{
		{"empty order", nil, domain.ErrEmptyOrder},
		{"zero quantity", items("blue", 0), domain.ErrInvalidQuantity},
		{"negative quantity", items("blue", -3), domain.ErrInvalidQuantity},
		{"quantity above the limit", items("blue", MaxQuantityPerLine+1), domain.ErrInvalidQuantity},
		{"duplicates that add up above the limit", items("blue", 600, "blue", 600), domain.ErrInvalidQuantity},
		// 4 x 2^62 wraps to 0, so the merged total would be a harmless-looking 5 without the per-item bound.
		{"quantities that wrap around to a small total", items("blue", 1<<62, "blue", 1<<62, "blue", 1<<62, "blue", 1<<62, "blue", 5), domain.ErrInvalidQuantity},
		{"unknown product", items("black", 1), domain.ErrUnknownProduct},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.uc.Execute(context.Background(), CalculateInput{Items: tt.items})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if len(f.orders.saved) != 0 || len(f.limiter.claims) != 0 {
				t.Fatal("an invalid request must not save an order or claim a slot")
			}
		})
	}
}

func TestCalculateOrder_RejectsUnknownOrSuspendedCardWithoutClaimingRed(t *testing.T) {
	for _, card := range []string{"NOPE", "MEM0002"} {
		t.Run(card, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.uc.Execute(context.Background(), CalculateInput{
				MemberCard: card, Items: items("red", 1),
			})
			if !errors.Is(err, domain.ErrInvalidMemberCard) {
				t.Fatalf("err = %v, want ErrInvalidMemberCard", err)
			}
			if len(f.limiter.claims) != 0 || len(f.orders.saved) != 0 {
				t.Fatal("a rejected card must not claim the Red slot or save an order")
			}
		})
	}
}

func TestCalculateOrder_LimiterOutageFallsBackToThePostgresGuard(t *testing.T) {
	f := newFixture(t)
	f.limiter.err = errors.New("redis: connection refused")
	f.orders.guarded = []domain.ProductID{"red"}

	q, err := f.uc.Execute(context.Background(), CalculateInput{ClientKey: "a", Items: items("red", 1)})
	if err != nil || q.FinalTotal == 0 {
		t.Fatalf("quote=%+v err=%v, want the order to succeed through the DB guard", q, err)
	}
	if len(f.orders.saved) != 1 || f.orders.guard["red"] != "a" {
		t.Fatal("the order must be saved with the DB guard holding red")
	}
	if len(f.limiter.released) != 0 {
		t.Fatalf("released=%v", f.limiter.released)
	}
}

func TestCalculateOrder_LimiterOutageWithHeldGuardIsASlotHeldErrorNotInternal(t *testing.T) {
	f := newFixture(t)
	f.limiter.err = errors.New("redis: connection refused")
	f.orders.guarded = []domain.ProductID{"red"}
	f.orders.guard = map[domain.ProductID]string{"red": "winner"}

	_, err := f.uc.Execute(context.Background(), CalculateInput{ClientKey: "late", Items: items("red", 1)})
	var held *domain.SlotHeldError
	if !errors.As(err, &held) || held.ProductName != "Red set" {
		t.Fatalf("err = %v, want *SlotHeldError", err)
	}
	if len(f.orders.saved) != 0 {
		t.Fatal("nothing may be saved")
	}
}

func TestCalculateOrder_RedisAdmitsButGuardHoldsReleasesTheRedisClaim(t *testing.T) {
	f := newFixture(t) // Redis lost its data: it admits a late holder
	f.orders.guarded = []domain.ProductID{"red"}
	f.orders.guard = map[domain.ProductID]string{"red": "winner"}

	_, err := f.uc.Execute(context.Background(), CalculateInput{ClientKey: "late", Items: items("red", 1)})
	var held *domain.SlotHeldError
	if !errors.As(err, &held) {
		t.Fatalf("err = %v, want *SlotHeldError", err)
	}
	if len(f.limiter.held) != 0 {
		t.Fatalf("the Redis claim must be released, held = %v", f.limiter.held)
	}
}

func TestCalculateOrder_AfterOneLimiterErrorTheLimiterIsNotCalledAgain(t *testing.T) {
	f := newFixture(t)
	f.limits.limits = map[domain.ProductID]time.Duration{"red": time.Hour, "pink": time.Hour}
	f.limiter.err = errors.New("redis down")
	calls := &countingLimiter{fakeLimiter: f.limiter}
	f.uc = NewCalculateOrder(CalculateOrderDeps{
		Products: f.products, Members: f.members, Rules: f.rules, Limits: f.limits,
		Orders: f.orders, Limiter: calls, Clock: fixedClock{testNow}, IDs: &seqIDs{},
	})

	if _, err := f.uc.Execute(context.Background(), CalculateInput{Items: items("red", 1, "pink", 1)}); err != nil {
		t.Fatal(err)
	}
	if calls.claimCalls != 1 {
		t.Fatalf("limiter Claim called %d times, want 1", calls.claimCalls)
	}
}

type countingLimiter struct {
	*fakeLimiter
	claimCalls int
}

func (c *countingLimiter) Claim(ctx context.Context, p domain.ProductID, h string, ttl time.Duration) (bool, error) {
	c.claimCalls++
	return c.fakeLimiter.Claim(ctx, p, h, ttl)
}

func TestCalculateOrder_SaveFailureIsReturned(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("db down")
	f.orders.err = boom

	if _, err := f.uc.Execute(context.Background(), CalculateInput{Items: items("blue", 1)}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestCalculateOrder_ProductListFailureIsReturned(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("db down")
	f.products.err = boom

	if _, err := f.uc.Execute(context.Background(), CalculateInput{Items: items("blue", 1)}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestCalculateOrder_SaveFailureReleasesTheRedClaim(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("db down")
	f.orders.err = boom
	ctx := context.Background()

	if _, err := f.uc.Execute(ctx, CalculateInput{Items: items("red", 1)}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if len(f.limiter.held) != 0 {
		t.Fatalf("a failed save must not leave Red held, held = %v", f.limiter.held)
	}

	f.orders.err = nil
	if _, err := f.uc.Execute(ctx, CalculateInput{Items: items("red", 1)}); err != nil {
		t.Fatalf("Red should be orderable after a failed save: %v", err)
	}
}

func TestCalculateOrder_SaveFailureKeepsAClaimTheCustomerAlreadyHad(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.uc.Execute(ctx, CalculateInput{ClientKey: "browser-1", Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}

	f.orders.err = errors.New("db down")
	if _, err := f.uc.Execute(ctx, CalculateInput{ClientKey: "browser-1", Items: items("red", 1)}); err == nil {
		t.Fatal("want the save error")
	}
	if f.limiter.held["red"] != "browser-1" {
		t.Fatal("the earlier, successful order's claim must survive a later failed re-price")
	}
}

func TestCalculateOrder_ReleaseNeverFreesAnotherHoldersClaim(t *testing.T) {
	f := newFixture(t)
	f.orders.err = errors.New("db down")
	f.limiter.held = map[domain.ProductID]string{"red": "somebody-else"}

	if _, err := f.uc.Execute(context.Background(), CalculateInput{Items: items("red", 1)}); !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want ErrSlotHeld", err)
	}
	if f.limiter.held["red"] != "somebody-else" {
		t.Fatal("another holder's claim must be left alone")
	}
}

func TestCalculateOrder_SameClientKeyCanPriceRedAgain(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.uc.Execute(ctx, CalculateInput{ClientKey: "browser-1", Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.uc.Execute(ctx, CalculateInput{ClientKey: "browser-1", Items: items("red", 2)}); err != nil {
		t.Fatalf("the holder's own repeat calculate must not be rejected: %v", err)
	}
	_, err := f.uc.Execute(ctx, CalculateInput{ClientKey: "browser-2", Items: items("red", 1)})
	if !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want ErrSlotHeld for a different client key", err)
	}
}

func TestCalculateOrder_WithoutAClientKeyEveryOrderIsADifferentHolder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.uc.Execute(ctx, CalculateInput{Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.uc.Execute(ctx, CalculateInput{Items: items("red", 1)}); !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want ErrSlotHeld", err)
	}
}

func TestCalculateOrder_RejectedLaterProductReleasesTheEarlierClaim(t *testing.T) {
	f := newFixture(t)
	f.limits.limits["yellow"] = time.Hour
	f.limiter.held = map[domain.ProductID]string{"yellow": "somebody-else"}

	// "red" sorts before "yellow", so it is claimed first and must be given back.
	_, err := f.uc.Execute(context.Background(), CalculateInput{Items: items("red", 1, "yellow", 1)})

	var held *domain.SlotHeldError
	if !errors.As(err, &held) || held.ProductName != "Yellow set" {
		t.Fatalf("err = %v, want SlotHeldError for Yellow set", err)
	}
	if _, stillHeld := f.limiter.held["red"]; stillHeld {
		t.Fatal("Red was claimed for an order that was rejected, so it must be released")
	}
}
