package postgres

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
)

type LimitRepository struct{ db *gorm.DB }

func NewLimitRepository(db *gorm.DB) *LimitRepository { return &LimitRepository{db: db} }

func (r *LimitRepository) ActiveLimits(ctx context.Context) (map[domain.ProductID]time.Duration, error) {
	var rows []productLimitModel
	if err := r.db.WithContext(ctx).Where("active").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query limits: %w", err)
	}
	out := make(map[domain.ProductID]time.Duration, len(rows))
	for _, l := range rows {
		out[domain.ProductID(l.ProductID)] = time.Duration(l.WindowSeconds) * time.Second
	}
	return out, nil
}
