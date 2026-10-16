package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/domain"
)

type LimitRepository struct{ pool *pgxpool.Pool }

func NewLimitRepository(pool *pgxpool.Pool) *LimitRepository { return &LimitRepository{pool: pool} }

func (r *LimitRepository) ActiveLimits(ctx context.Context) (map[domain.ProductID]time.Duration, error) {
	rows, err := r.pool.Query(ctx, "SELECT product_id, window_seconds FROM product_limits WHERE active")
	if err != nil {
		return nil, fmt.Errorf("query limits: %w", err)
	}
	defer rows.Close()

	out := map[domain.ProductID]time.Duration{}
	for rows.Next() {
		var id string
		var seconds int
		if err := rows.Scan(&id, &seconds); err != nil {
			return nil, fmt.Errorf("scan limit: %w", err)
		}
		out[domain.ProductID(id)] = time.Duration(seconds) * time.Second
	}
	return out, rows.Err()
}
