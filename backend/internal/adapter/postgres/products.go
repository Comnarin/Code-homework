package postgres

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/domain"
)

type ProductRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{
		db: db,
	}
}

func (r *ProductRepository) List(ctx context.Context) ([]domain.Product, error) {
	var rows []productModel
	if err := r.db.WithContext(ctx).Order("sort_order").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	out := make([]domain.Product, len(rows))
	for i, p := range rows {
		out[i] = toProduct(p)
	}
	return out, nil
}

func (r *ProductRepository) FindByID(ctx context.Context, id domain.ProductID) (domain.Product, bool, error) {
	var rows []productModel
	if err := r.db.WithContext(ctx).Where("id = ?", string(id)).Limit(1).Find(&rows).Error; err != nil {
		return domain.Product{}, false, fmt.Errorf("query product: %w", err)
	}
	if len(rows) == 0 {
		return domain.Product{}, false, nil
	}
	return toProduct(rows[0]), true, nil
}

func toProduct(p productModel) domain.Product {
	out := domain.Product{ID: domain.ProductID(p.ID), Name: p.Name, Price: domain.Money(p.PriceSatang)}
	if p.ImageKey != nil {
		out.ImageKey = *p.ImageKey
	}
	return out
}
