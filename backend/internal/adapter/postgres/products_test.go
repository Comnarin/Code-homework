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

func TestProductRepository_ListCarriesTheImageKey(t *testing.T) {
	gdb := testDB(t)
	if err := gdb.Exec("UPDATE products SET image_key = 'red.png' WHERE id = 'red'").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { gdb.Exec("UPDATE products SET image_key = NULL WHERE id = 'red'") })

	got, err := NewProductRepository(gdb).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]string{}
	for _, p := range got {
		byID[string(p.ID)] = p.ImageKey
	}
	if byID["red"] != "red.png" {
		t.Fatalf("red image key = %q, want red.png", byID["red"])
	}
	if byID["blue"] != "" {
		t.Fatalf("a product with no image must have an empty key, got %q", byID["blue"])
	}
}

func TestProductRepository_FindByID(t *testing.T) {
	repo := NewProductRepository(testDB(t))

	got, found, err := repo.FindByID(context.Background(), "orange")
	if err != nil || !found || got.Name != "Orange set" || got.Price != 12000 {
		t.Fatalf("got %+v found=%v err=%v", got, found, err)
	}

	_, found, err = repo.FindByID(context.Background(), "nope")
	if err != nil || found {
		t.Fatalf("an unknown product is not found and not an error: found=%v err=%v", found, err)
	}
}
