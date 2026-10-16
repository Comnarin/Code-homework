package usecase

import (
	"context"
	"errors"
	"math"
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
	pair, err := domain.NewBundlePercent("PAIR_5", 10, []domain.ProductID{"orange", "pink", "green"}, 2, 500)
	if err != nil {
		t.Fatal(err)
	}
	member, err := domain.NewMemberPercent("MEMBER_10", 100, 1000)
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

	q, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("orange", 2)})
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
	if o.ID != "order-1" || o.CustomerID != "cust-1" || !o.CreatedAt.Equal(testNow) || o.MemberCard != "" {
		t.Fatalf("saved order wrong: %+v", o)
	}
	if len(o.Lines) != 1 || o.Lines[0].Product.ID != "orange" || o.Lines[0].Quantity != 2 {
		t.Fatalf("saved lines wrong: %+v", o.Lines)
	}
}

func TestCalculateOrder_AppliesMemberDiscountAfterPairDiscount(t *testing.T) {
	f := newFixture(t)

	// Card is entered messily on purpose: the use case normalizes it.
	q, err := f.uc.Execute(context.Background(), CalculateInput{
		CustomerID: "cust-1", MemberCard: "  mem0001 ", Items: items("orange", 2),
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
		CustomerID: "cust-1", Items: items("orange", 1, "orange", 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if q.FinalTotal != 22800 { // one pair discount, as if orange x2
		t.Fatalf("FinalTotal = %d, want 22800", q.FinalTotal)
	}
	if len(f.orders.saved[0].Lines) != 1 {
		t.Fatalf("duplicate lines should merge, got %+v", f.orders.saved[0].Lines)
	}
}

func TestCalculateOrder_SameCustomerMayOrderRedTwiceInTheHour(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-1", Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}
	q, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-1", MemberCard: "MEM0001", Items: items("red", 1)})
	if err != nil {
		t.Fatalf("second Red order by the same customer should be allowed: %v", err)
	}
	if q.FinalTotal != 4500 { // 5000 - 10% member
		t.Fatalf("FinalTotal = %d, want 4500", q.FinalTotal)
	}
	if len(f.orders.saved) != 2 {
		t.Fatalf("want 2 saved orders, got %d", len(f.orders.saved))
	}
}

func TestCalculateOrder_OnlyClaimsSlotsForLimitedProducts(t *testing.T) {
	f := newFixture(t)

	if _, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("blue", 1, "red", 1)}); err != nil {
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
		{"quantities that would overflow when merged", items("blue", math.MaxInt, "blue", 2), domain.ErrInvalidQuantity},
		{"unknown product", items("black", 1), domain.ErrUnknownProduct},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			_, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: tt.items})
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
				CustomerID: "cust-1", MemberCard: card, Items: items("red", 1),
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

func TestCalculateOrder_DifferentCustomerCannotOrderRedWhileHeld(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-1", Items: items("red", 1)}); err != nil {
		t.Fatal(err)
	}

	_, err := f.uc.Execute(ctx, CalculateInput{CustomerID: "cust-2", MemberCard: "MEM0001", Items: items("red", 1)})

	var held *domain.SlotHeldError
	if !errors.As(err, &held) || held.ProductName != "Red set" || !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want SlotHeldError for Red set", err)
	}
	if len(f.orders.saved) != 1 {
		t.Fatalf("the rejected order must not be saved, saved = %d", len(f.orders.saved))
	}
}

func TestCalculateOrder_LimiterOutageFailsTheRequestAndSavesNothing(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("redis: connection refused")
	f.limiter.err = boom

	_, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("red", 1)})

	if !errors.Is(err, boom) || errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want the raw limiter error (not ErrSlotHeld)", err)
	}
	if len(f.orders.saved) != 0 {
		t.Fatal("no order may be saved when the limiter fails")
	}
}

func TestCalculateOrder_SaveFailureIsReturned(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("db down")
	f.orders.err = boom

	if _, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("blue", 1)}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestCalculateOrder_ProductListFailureIsReturned(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("db down")
	f.products.err = boom

	if _, err := f.uc.Execute(context.Background(), CalculateInput{CustomerID: "cust-1", Items: items("blue", 1)}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
