package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type OrderRepository struct{ pool *pgxpool.Pool }

func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository { return &OrderRepository{pool: pool} }

// Save writes the order, its items and its discounts in one transaction.
func (r *OrderRepository) Save(ctx context.Context, o domain.Order) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op

	var card *string
	if o.MemberCard != "" {
		card = &o.MemberCard
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO orders (id, customer_id, member_card, total_before_satang, member_discount_satang, final_total_satang, created_at)
		 VALUES ($1::text::uuid, $2, $3, $4, $5, $6, $7)`,
		o.ID, o.CustomerID, card, int64(o.Quote.TotalBefore), int64(o.Quote.MemberDiscount()), int64(o.Quote.FinalTotal), o.CreatedAt); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}
	for _, l := range o.Lines {
		if _, err := tx.Exec(ctx,
			"INSERT INTO order_items (order_id, product_id, quantity) VALUES ($1::text::uuid, $2, $3)",
			o.ID, string(l.Product.ID), l.Quantity); err != nil {
			return fmt.Errorf("insert item %s: %w", l.Product.ID, err)
		}
	}
	for _, d := range o.Quote.Discounts {
		var productID *string
		if d.ProductID != "" {
			id := string(d.ProductID)
			productID = &id
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO order_discounts (order_id, rule_code, product_id, amount_satang) VALUES ($1::text::uuid, $2, $3, $4)",
			o.ID, d.RuleCode, productID, int64(d.Amount)); err != nil {
			return fmt.Errorf("insert discount %s: %w", d.RuleCode, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
