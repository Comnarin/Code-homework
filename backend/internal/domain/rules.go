package domain

import "fmt"

var (
	_ Rule = BundlePercent{}
	_ Rule = MemberPercent{}
)

// BundlePercent gives percentBP off every complete group of `size` sets of the
// same eligible product (for example every pair of Orange).
type BundlePercent struct {
	code      string
	priority  int
	eligible  map[ProductID]struct{}
	size      int
	percentBP int
}

func NewBundlePercent(code string, priority int, products []ProductID, size, percentBP int) (BundlePercent, error) {
	switch {
	case code == "":
		return BundlePercent{}, fmt.Errorf("%w: code is required", ErrInvalidRule)
	case len(products) == 0:
		return BundlePercent{}, fmt.Errorf("%w: %s needs at least one product", ErrInvalidRule, code)
	case size < 2:
		return BundlePercent{}, fmt.Errorf("%w: %s bundle size must be at least 2", ErrInvalidRule, code)
	case percentBP < 1 || percentBP > 10000:
		return BundlePercent{}, fmt.Errorf("%w: %s percent must be between 1 and 10000 bp", ErrInvalidRule, code)
	}
	eligible := make(map[ProductID]struct{}, len(products))
	for _, p := range products {
		eligible[p] = struct{}{}
	}
	return BundlePercent{code: code, priority: priority, eligible: eligible, size: size, percentBP: percentBP}, nil
}

func (r BundlePercent) Code() string  { return r.code }
func (r BundlePercent) Priority() int { return r.priority }

func (r BundlePercent) Apply(lines []Line, _ Money, _ bool) []Discount {
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
			Amount:    base.ApplyBP(r.percentBP),
		})
	}
	return out
}

// MemberPercent gives percentBP off the running subtotal to members.
type MemberPercent struct {
	code      string
	priority  int
	percentBP int
}

func NewMemberPercent(code string, priority, percentBP int) (MemberPercent, error) {
	switch {
	case code == "":
		return MemberPercent{}, fmt.Errorf("%w: code is required", ErrInvalidRule)
	case percentBP < 1 || percentBP > 10000:
		return MemberPercent{}, fmt.Errorf("%w: %s percent must be between 1 and 10000 bp", ErrInvalidRule, code)
	}
	return MemberPercent{code: code, priority: priority, percentBP: percentBP}, nil
}

func (r MemberPercent) Code() string  { return r.code }
func (r MemberPercent) Priority() int { return r.priority }

func (r MemberPercent) Apply(_ []Line, subtotal Money, isMember bool) []Discount {
	if !isMember {
		return nil
	}
	amount := subtotal.ApplyBP(r.percentBP)
	if amount == 0 {
		return nil
	}
	return []Discount{{RuleCode: r.code, Scope: ScopeMember, Amount: amount}}
}
