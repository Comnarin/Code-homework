package usecase

import (
	"context"
	"fmt"

	"github.com/comnarin/foodstore/internal/domain"
)

type Products struct {
	products ProductRepository
}

func NewProducts(products ProductRepository) *Products {
	return &Products{
		products: products,
	}
}

func (u *Products) List(ctx context.Context) ([]domain.Product, error) {
	items, err := u.products.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return items, nil
}
