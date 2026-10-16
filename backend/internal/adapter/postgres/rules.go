package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type RuleRepository struct{ pool *pgxpool.Pool }

func NewRuleRepository(pool *pgxpool.Pool) *RuleRepository { return &RuleRepository{pool: pool} }

type ruleRow struct {
	code       string
	typ        string
	percentBP  int
	bundleSize *int
	priority   int
	products   []string
}

const activeRulesSQL = `
SELECT r.code, r.type, r.percent_bp, r.bundle_size, r.priority,
       COALESCE(array_agg(rp.product_id) FILTER (WHERE rp.product_id IS NOT NULL), '{}')
FROM rules r
LEFT JOIN rule_products rp ON rp.rule_id = r.id
WHERE r.active
GROUP BY r.id
ORDER BY r.priority, r.id`

func (r *RuleRepository) ActiveRules(ctx context.Context) ([]domain.Rule, error) {
	rows, err := r.pool.Query(ctx, activeRulesSQL)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	var out []domain.Rule
	for rows.Next() {
		var row ruleRow
		if err := rows.Scan(&row.code, &row.typ, &row.percentBP, &row.bundleSize, &row.priority, &row.products); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		rule, err := buildRule(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
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
