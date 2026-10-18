package domain

import "sort"

type PromotionKind string

const (
	PromotionBundle PromotionKind = "BUNDLE_PERCENT"
	PromotionMember PromotionKind = "MEMBER_PERCENT"
)

// Promotion is a read-only description of a rule's parameters, so clients can
// show the live rules instead of hard-coding them.
type Promotion struct {
	Code       string
	Kind       PromotionKind
	Percent    int
	BundleSize int         // BundlePercent only
	ProductIDs []ProductID // BundlePercent only, sorted
}

// Describer is implemented by rules that can describe themselves.
type Describer interface{ Describe() Promotion }

var (
	_ Describer = BundlePercent{}
	_ Describer = MemberPercent{}
)

func (r BundlePercent) Describe() Promotion {
	ids := make([]ProductID, 0, len(r.eligible))
	for id := range r.eligible {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return Promotion{Code: r.code, Kind: PromotionBundle, Percent: r.percent, BundleSize: r.size, ProductIDs: ids}
}

func (r MemberPercent) Describe() Promotion {
	return Promotion{Code: r.code, Kind: PromotionMember, Percent: r.percent}
}
