package domain

import "fmt"

var (
	_ Rule = BundlePercent{}
	_ Rule = MemberPercent{}
)

// BundlePercent gives percent off every complete group of `size` sets of the
// same eligible product (for example every pair of Orange).
type BundlePercent struct {
	code     string
	priority int
	eligible map[ProductID]struct{}
	size     int
	percent  int
}

func NewBundlePercent(code string, priority int, products []ProductID, size, percent int) (BundlePercent, error) {
	switch {
	case code == "":
		return BundlePercent{}, fmt.Errorf("%w: code is required", ErrInvalidRule)
	case len(products) == 0:
		return BundlePercent{}, fmt.Errorf("%w: %s needs at least one product", ErrInvalidRule, code)
	case size < 2:
		return BundlePercent{}, fmt.Errorf("%w: %s bundle size must be at least 2", ErrInvalidRule, code)
	case percent < 1 || percent > 100:
		return BundlePercent{}, fmt.Errorf("%w: %s percent must be between 1 and 100", ErrInvalidRule, code)
	}
	eligible := make(map[ProductID]struct{}, len(products))
	for _, p := range products {
		eligible[p] = struct{}{}
	}
	return BundlePercent{code: code, priority: priority, eligible: eligible, size: size, percent: percent}, nil
}

func (r BundlePercent) Code() string  { return r.code }
func (r BundlePercent) Priority() int { return r.priority }

func (r BundlePercent) Apply(lines []Cart, _ Money, _ bool) []Discount {
	var out []Discount
	for _, l := range lines {
		if _, ok := r.eligible[l.Product.ID]; !ok {
			continue
		}
		bundles := l.Quantity / r.size
		if bundles == 0 {
			continue
		}
		base := l.Product.Price * Money(bundles*r.size)
		out = append(out, Discount{
			RuleCode:  r.code,
			Scope:     ScopeItem,
			ProductID: l.Product.ID,
			Name:      l.Product.Name,
			Bundles:   bundles,
			Base:      base,
			Amount:    base.ApplyPercent(r.percent),
		})
	}
	return out
}

// MemberPercent gives percent off the running subtotal to members.
type MemberPercent struct {
	code     string
	priority int
	percent  int
}

func NewMemberPercent(code string, priority, percent int) (MemberPercent, error) {
	switch {
	case code == "":
		return MemberPercent{}, fmt.Errorf("%w: code is required", ErrInvalidRule)
	case percent < 1 || percent > 100:
		return MemberPercent{}, fmt.Errorf("%w: %s percent must be between 1 and 100", ErrInvalidRule, code)
	}
	return MemberPercent{code: code, priority: priority, percent: percent}, nil
}

func (r MemberPercent) Code() string  { return r.code }
func (r MemberPercent) Priority() int { return r.priority }

func (r MemberPercent) Apply(_ []Cart, subtotal Money, isMember bool) []Discount {
	if !isMember {
		return nil
	}
	amount := subtotal.ApplyPercent(r.percent)
	if amount == 0 {
		return nil
	}
	return []Discount{{RuleCode: r.code, Scope: ScopeMember, Base: subtotal, Amount: amount}}
}
