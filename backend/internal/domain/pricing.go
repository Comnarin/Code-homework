package domain

import "sort"

// Scope says what a discount applies to.
type Scope int

const (
	ScopeItem   Scope = iota // a specific product line
	ScopeMember              // the whole order, because the customer is a member
)

// Discount is one amount taken off an order by one rule.
type Discount struct {
	RuleCode  string
	Scope     Scope
	ProductID ProductID // empty for ScopeMember
	Name      string
	Bundles   int // how many groups (for example pairs) were discounted
	Amount    Money
}

// Rule is a pricing condition. Rules never mutate their inputs.
type Rule interface {
	Code() string
	Priority() int // lower runs first
	// Apply returns the discounts this rule grants. subtotal is the running
	// total after earlier rules.
	Apply(lines []Cart, subtotal Money, isMember bool) []Discount
}

// Quote is the priced result of an order.
type Quote struct {
	Lines       []Cart // what was ordered, in order; a copy, safe to keep
	TotalBefore Money
	Discounts   []Discount
	FinalTotal  Money
}

func (q Quote) ItemDiscounts() []Discount {
	var out []Discount
	for _, d := range q.Discounts {
		if d.Scope == ScopeItem {
			out = append(out, d)
		}
	}
	return out
}

func (q Quote) MemberDiscount() Money {
	var sum Money
	for _, d := range q.Discounts {
		if d.Scope == ScopeMember {
			sum += d.Amount
		}
	}
	return sum
}

// Calculate prices lines with rules. It is pure: same input, same output.
// Lines must have distinct products; the caller merges duplicates.
func Calculate(lines []Cart, isMember bool, rules []Rule) (Quote, error) {
	if len(lines) == 0 {
		return Quote{}, ErrEmptyOrder
	}
	var total Money
	for _, l := range lines {
		if l.Quantity <= 0 {
			return Quote{}, ErrInvalidQuantity
		}
		total += l.Product.Price * Money(l.Quantity)
	}

	ordered := append([]Rule(nil), rules...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Priority() < ordered[j].Priority() })

	q := Quote{Lines: append([]Cart(nil), lines...), TotalBefore: total, FinalTotal: total}
	for _, r := range ordered {
		for _, d := range r.Apply(lines, q.FinalTotal, isMember) {
			// Never take off more than is left, so the total cannot go negative.
			if d.Amount > q.FinalTotal {
				d.Amount = q.FinalTotal
			}
			if d.Amount <= 0 {
				continue
			}
			q.Discounts = append(q.Discounts, d)
			q.FinalTotal -= d.Amount
		}
	}
	return q, nil
}
