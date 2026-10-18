package usecase

import (
	"context"
	"testing"

	"github.com/comnarin/foodstore/internal/domain"
)

func TestListPromotions_DescribesEveryActiveRule(t *testing.T) {
	pair, _ := domain.NewBundlePercent("PAIR_5", 10, []domain.ProductID{"green"}, 2, 5)
	member, _ := domain.NewMemberPercent("MEMBER_10", 100, 10)
	uc := NewPromotions(&fakeRules{rules: []domain.Rule{pair, member}})

	got, err := uc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Code != "PAIR_5" || got[1].Percent != 10 {
		t.Fatalf("got %+v", got)
	}
}
