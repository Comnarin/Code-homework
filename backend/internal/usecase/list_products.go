package usecase

import (
	"context"
	"fmt"

	"github.com/comnarin/foodstore/internal/domain"
)

type ListProducts struct{ products ProductRepository }

func NewListProducts(products ProductRepository) *ListProducts {
	return &ListProducts{products: products}
}

func (u *ListProducts) Execute(ctx context.Context) ([]domain.Product, error) {
	items, err := u.products.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	return items, nil
}
