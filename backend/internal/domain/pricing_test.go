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
func (s stubRule) Apply(_ []Line, subtotal Money, _ bool) []Discount {
	if s.gotSubtotal != nil {
		*s.gotSubtotal = subtotal
	}
	return []Discount{{RuleCode: s.code, Scope: s.scope, Amount: s.amount}}
}

func TestCalculate_NoRules_TotalsTheLines(t *testing.T) {
	q, err := Calculate([]Line{{blue, 2}, {yellow, 1}}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if q.TotalBefore != 11000 || q.FinalTotal != 11000 || len(q.Discounts) != 0 {
		t.Fatalf("got %+v", q)
	}
}

func TestCalculate_RejectsBadOrders(t *testing.T) {
	tests := []struct {
		name  string
		lines []Line
		want  error
	}{
		{"empty order", nil, ErrEmptyOrder},
		{"zero quantity", []Line{{blue, 0}}, ErrInvalidQuantity},
		{"negative quantity", []Line{{blue, -1}}, ErrInvalidQuantity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Calculate(tt.lines, false, nil); !errors.Is(err, tt.want) {
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
	q, err := Calculate([]Line{{blue, 10}}, true, []Rule{late, early})
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
	if _, err := Calculate([]Line{{blue, 1}}, false, rules); err != nil {
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
