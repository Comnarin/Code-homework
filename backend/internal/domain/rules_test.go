package domain

import (
	"errors"
	"testing"
)

func pairRule(t *testing.T) BundlePercent {
	t.Helper()
	r, err := NewBundlePercent("PAIR_5", 10, []ProductID{"orange", "pink", "green"}, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBundlePercent_Apply(t *testing.T) {
	tests := []struct {
		name        string
		cart        []Cart
		wantBundles int
		wantAmount  Money
		wantNone    bool
	}{
		{"one orange is not a pair", []Cart{{orange, 1}}, 0, 0, true},
		{"orange x2 is one pair", []Cart{{orange, 2}}, 1, 1200, false},
		{"pink x4 is two pairs", []Cart{{pink, 4}}, 2, 1600, false},
		{"green x3 is one pair plus a full-price set", []Cart{{green, 3}}, 1, 400, false},
		{"ineligible product gets nothing", []Cart{{blue, 6}}, 0, 0, true},
		{"red gets nothing", []Cart{{red, 2}}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pairRule(t).Apply(tt.cart, 0, false)
			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("want no discounts, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Bundles != tt.wantBundles || got[0].Amount != tt.wantAmount {
				t.Fatalf("got %+v, want bundles=%d amount=%d", got, tt.wantBundles, tt.wantAmount)
			}
			if got[0].Scope != ScopeItem || got[0].RuleCode != "PAIR_5" {
				t.Fatalf("scope or code wrong: %+v", got[0])
			}
		})
	}
}

func TestBundlePercent_MixedColoursDoNotPairWithEachOther(t *testing.T) {
	got := pairRule(t).Apply([]Cart{{orange, 1}, {pink, 1}}, 0, false)
	if len(got) != 0 {
		t.Fatalf("one orange plus one pink must not form a pair, got %+v", got)
	}
}

func TestBundlePercent_DiscountsEachEligibleLineSeparately(t *testing.T) {
	got := pairRule(t).Apply([]Cart{{orange, 2}, {pink, 2}}, 0, false)
	if len(got) != 2 || got[0].ProductID != "orange" || got[1].ProductID != "pink" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Name != "Orange set" {
		t.Fatalf("discount should carry the product name, got %q", got[0].Name)
	}
}

func TestNewBundlePercent_RejectsInvalidConfig(t *testing.T) {
	ok := []ProductID{"orange"}
	tests := []struct {
		name     string
		code     string
		products []ProductID
		size     int
		percent  int
	}{
		{"empty code", "", ok, 2, 5},
		{"no products", "X", nil, 2, 5},
		{"size below 2", "X", ok, 1, 5},
		{"percent zero", "X", ok, 2, 0},
		{"percent above 100", "X", ok, 2, 101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBundlePercent(tt.code, 10, tt.products, tt.size, tt.percent)
			if !errors.Is(err, ErrInvalidRule) {
				t.Fatalf("err = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func TestMemberPercent_Apply(t *testing.T) {
	r, err := NewMemberPercent("MEMBER_10", 100, 10)
	if err != nil {
		t.Fatal(err)
	}

	if got := r.Apply(nil, 22800, false); len(got) != 0 {
		t.Fatalf("non-member should get nothing, got %+v", got)
	}
	got := r.Apply(nil, 22800, true)
	if len(got) != 1 || got[0].Amount != 2280 || got[0].Scope != ScopeMember || got[0].RuleCode != "MEMBER_10" {
		t.Fatalf("got %+v", got)
	}
	if got := r.Apply(nil, 0, true); len(got) != 0 {
		t.Fatalf("zero subtotal should yield no discount, got %+v", got)
	}
}

func TestNewMemberPercent_RejectsInvalidConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		code    string
		percent int
	}{
		"empty code":    {"", 10},
		"percent zero":  {"X", 0},
		"percent > 100": {"X", 101},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewMemberPercent(tc.code, 100, tc.percent); !errors.Is(err, ErrInvalidRule) {
				t.Fatalf("err = %v, want ErrInvalidRule", err)
			}
		})
	}
}

func TestDescribe_ExposesRuleParametersSoClientsNeverHardCodeThem(t *testing.T) {
	bundle, err := NewBundlePercent("PAIR_5", 10, []ProductID{"yellow", "orange"}, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	got := bundle.Describe()
	if got.Kind != PromotionBundle || got.Percent != 5 || got.BundleSize != 2 ||
		len(got.ProductIDs) != 2 || got.ProductIDs[0] != "orange" || got.ProductIDs[1] != "yellow" {
		t.Fatalf("bundle = %+v", got)
	}

	member, err := NewMemberPercent("MEMBER_10", 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := member.Describe(); got.Kind != PromotionMember || got.Percent != 10 {
		t.Fatalf("member = %+v", got)
	}
}
