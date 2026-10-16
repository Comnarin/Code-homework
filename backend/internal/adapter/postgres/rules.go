package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
)

type RuleRepository struct{ db *gorm.DB }

func NewRuleRepository(db *gorm.DB) *RuleRepository { return &RuleRepository{db: db} }

// ruleRow is the shape buildRule needs; it is independent of GORM so the
// mapping can be unit-tested without a database.
type ruleRow struct {
	code       string
	typ        string
	percentBP  int
	bundleSize *int
	priority   int
	products   []string
}

func (r *RuleRepository) ActiveRules(ctx context.Context) ([]domain.Rule, error) {
	var rows []ruleModel
	if err := r.db.WithContext(ctx).
		Preload("Products").
		Where("active").
		Order("priority, id").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}

	out := make([]domain.Rule, 0, len(rows))
	for _, m := range rows {
		products := make([]string, len(m.Products))
		for i, p := range m.Products {
			products[i] = p.ProductID
		}
		rule, err := buildRule(ruleRow{
			code: m.Code, typ: m.Type, percentBP: m.PercentBP,
			bundleSize: m.BundleSize, priority: m.Priority, products: products,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, nil
}

// buildRule maps a table row to a domain rule. The domain knows nothing about tables.
func buildRule(row ruleRow) (domain.Rule, error) {
	switch row.typ {
	case "BUNDLE_PERCENT":
		if row.bundleSize == nil {
			return nil, fmt.Errorf("%w: %s has no bundle_size", domain.ErrInvalidRule, row.code)
		}
		products := make([]domain.ProductID, len(row.products))
		for i, p := range row.products {
			products[i] = domain.ProductID(p)
		}
		return domain.NewBundlePercent(row.code, row.priority, products, *row.bundleSize, row.percentBP)
	case "MEMBER_PERCENT":
		return domain.NewMemberPercent(row.code, row.priority, row.percentBP)
	default:
		return nil, fmt.Errorf("%w: %s has unknown type %q", domain.ErrInvalidRule, row.code, row.typ)
	}
}
