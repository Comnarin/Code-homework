// Package postgres implements the use-case ports on Postgres via pgx.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type ProductRepository struct{ pool *pgxpool.Pool }

func NewProductRepository(pool *pgxpool.Pool) *ProductRepository {
	return &ProductRepository{pool: pool}
}

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, name, price_satang FROM products ORDER BY sort_order")
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()

	var out []domain.Product
	for rows.Next() {
		var p domain.Product
		var id string
		var price int64
		if err := rows.Scan(&id, &p.Name, &price); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		p.ID, p.Price = domain.ProductID(id), domain.Money(price)
		out = append(out, p)
	}
	return out, rows.Err()
}
