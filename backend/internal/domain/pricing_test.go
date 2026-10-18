package domain

import (
	"errors"
	"testing"
)

// stubRule is a scripted rule that records the subtotal it was given.
type stubRule struct {
	code        string
	priority    int
	scope       Scope
	amount      Money
	gotSubtotal *Money
}

func (s stubRule) Code() string  { return s.code }
func (s stubRule) Priority() int { return s.priority }
func (s stubRule) Apply(_ []Cart, subtotal Money, _ bool) []Discount {
	if s.gotSubtotal != nil {
		*s.gotSubtotal = subtotal
	}
	return []Discount{{RuleCode: s.code, Scope: s.scope, Amount: s.amount}}
}

func TestCalculate_NoRules_TotalsTheLines(t *testing.T) {
	q, err := Calculate([]Cart{{blue, 2}, {yellow, 1}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if q.TotalBefore != 11000 || q.FinalTotal != 11000 || len(q.Discounts) != 0 {
		t.Fatalf("got %+v", q)
	}
}

func TestCalculate_RejectsBadOrders(t *testing.T) {
	tests := []struct {
		name string
		cart []Cart
		want error
	}{
		{"empty order", nil, ErrEmptyOrder},
		{"zero quantity", []Cart{{blue, 0}}, ErrInvalidQuantity},
		{"negative quantity", []Cart{{blue, -1}}, ErrInvalidQuantity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Calculate(tt.cart, false, nil); !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCalculate_RunsRulesInPriorityOrderOnTheRunningSubtotal(t *testing.T) {
	var seenByLate Money
	early := stubRule{code: "EARLY", priority: 10, scope: ScopeItem, amount: 1000}
	late := stubRule{code: "LATE", priority: 100, scope: ScopeMember, amount: 500, gotSubtotal: &seenByLate}

	// Passed in the wrong order on purpose.
	q, err := Calculate([]Cart{{blue, 10}}, true, []Rule{late, early})
	if err != nil {
		t.Fatal(err)
	}
	if seenByLate != 29000 { // 30000 - 1000 from the earlier rule
		t.Fatalf("late rule saw subtotal %d, want 29000", seenByLate)
	}
	if q.FinalTotal != 28500 {
		t.Fatalf("FinalTotal = %d, want 28500", q.FinalTotal)
	}
	if q.Discounts[0].RuleCode != "EARLY" || q.Discounts[1].RuleCode != "LATE" {
		t.Fatalf("discount order wrong: %+v", q.Discounts)
	}
}

func TestCalculate_DoesNotReorderTheCallersSlice(t *testing.T) {
	rules := []Rule{
		stubRule{code: "B", priority: 20, amount: 1},
		stubRule{code: "A", priority: 10, amount: 1},
	}
	if _, err := Calculate([]Cart{{blue, 1}}, false, rules); err != nil {
		t.Fatal(err)
	}
	if rules[0].Code() != "B" {
		t.Fatal("Calculate must not mutate the rules slice it was given")
	}
}

func TestQuote_SplitsItemAndMemberDiscounts(t *testing.T) {
	q := Quote{Discounts: []Discount{
		{RuleCode: "PAIR_5", Scope: ScopeItem, ProductID: "pink", Amount: 1600},
		{RuleCode: "MEMBER_10", Scope: ScopeMember, Amount: 2000},
	}}
	if got := q.ItemDiscounts(); len(got) != 1 || got[0].RuleCode != "PAIR_5" {
		t.Fatalf("ItemDiscounts = %+v", got)
	}
	if got := q.MemberDiscount(); got != 2000 {
		t.Fatalf("MemberDiscount = %d, want 2000", got)
	}
}

func TestCalculate_QuoteListsTheOrderedLines(t *testing.T) {
	q, err := Calculate([]Cart{{orange, 2}, {blue, 1}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Lines) != 2 ||
		q.Lines[0].Product.ID != "orange" || q.Lines[0].Quantity != 2 ||
		q.Lines[1].Product.ID != "blue" || q.Lines[1].Quantity != 1 {
		t.Fatalf("Lines = %+v, want orange x2 then blue x1", q.Lines)
	}
}

func TestCalculate_QuoteLinesAreACopyOfTheInput(t *testing.T) {
	input := []Cart{{orange, 2}}
	q, err := Calculate(input, false, nil)
	if err != nil {
		t.Fatal(err)
	}

	input[0].Quantity = 99

	if q.Lines[0].Quantity != 2 {
		t.Fatal("changing the caller's slice afterwards must not change the quote")
	}
}

func TestCalculate_DiscountsNeverTakeTheTotalBelowZero(t *testing.T) {
	// Two rules that each want 100% of a 3000 order: the second gets only what is left.
	first := stubRule{code: "A", priority: 10, scope: ScopeItem, amount: 3000}
	second := stubRule{code: "B", priority: 20, scope: ScopeMember, amount: 3000}

	q, err := Calculate([]Cart{{blue, 1}}, true, []Rule{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if q.FinalTotal != 0 {
		t.Fatalf("FinalTotal = %d, want 0", q.FinalTotal)
	}
	if len(q.Discounts) != 1 || q.Discounts[0].RuleCode != "A" {
		t.Fatalf("a discount with nothing left to take off should be dropped, got %+v", q.Discounts)
	}
}

func TestCalculate_CapsADiscountAtWhatIsLeft(t *testing.T) {
	big := stubRule{code: "BIG", priority: 10, scope: ScopeItem, amount: 5000}

	q, err := Calculate([]Cart{{blue, 1}}, false, []Rule{big})
	if err != nil {
		t.Fatal(err)
	}
	if q.Discounts[0].Amount != 3000 || q.FinalTotal != 0 {
		t.Fatalf("got discount %d, final %d; want 3000 and 0", q.Discounts[0].Amount, q.FinalTotal)
	}
}
