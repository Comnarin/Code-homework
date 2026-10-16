package postgres

import (
	"context"
	"testing"

	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.ProductRepository = (*ProductRepository)(nil)

func TestProductRepository_ListReturnsMenuInDisplayOrder(t *testing.T) {
	repo := NewProductRepository(testDB(t))

	got, err := repo.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 || got[0].ID != "red" || got[0].Price != 5000 || got[6].ID != "orange" || got[6].Name != "Orange set" {
		t.Fatalf("got %+v", got)
	}
}
