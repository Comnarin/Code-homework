package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
)

type ProductRepository struct{ db *gorm.DB }

func NewProductRepository(db *gorm.DB) *ProductRepository { return &ProductRepository{db: db} }

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	var rows []productModel
	if err := r.db.WithContext(ctx).Order("sort_order").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	out := make([]domain.Product, len(rows))
	for i, p := range rows {
		out[i] = domain.Product{ID: domain.ProductID(p.ID), Name: p.Name, Price: domain.Money(p.PriceSatang)}
	}
	return out, nil
}
