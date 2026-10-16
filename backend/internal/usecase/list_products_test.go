package usecase

import (
	"context"
	"errors"
	"testing"
)

func TestListProducts_ReturnsTheCatalog(t *testing.T) {
	uc := NewListProducts(&fakeProducts{items: seedProducts()})

	got, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 7 || got[0].Name != "Red set" {
		t.Fatalf("got %+v", got)
	}
}

func TestListProducts_PropagatesRepositoryErrors(t *testing.T) {
	boom := errors.New("db down")
	uc := NewListProducts(&fakeProducts{err: boom})

	if _, err := uc.Execute(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
