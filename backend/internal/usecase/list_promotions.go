package usecase

import (
	"context"
	"fmt"

	"github.com/comnarin/foodstore/internal/domain"
)

// ListPromotions describes the active rules, so the UI never hard-codes them.
type ListPromotions struct{ rules RuleRepository }

func NewListPromotions(rules RuleRepository) *ListPromotions { return &ListPromotions{rules: rules} }

func (u *ListPromotions) Execute(ctx context.Context) ([]domain.Promotion, error) {
	rules, err := u.rules.ActiveRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("list promotions: %w", err)
	}
	out := make([]domain.Promotion, 0, len(rules))
	for _, r := range rules {
		if d, ok := r.(domain.Describer); ok {
			out = append(out, d.Describe())
		}
	}
	return out, nil
}
