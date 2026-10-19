package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/comnarin/foodstore/internal/domain"
)

type fakeImageStore struct {
	images map[string]domain.Image
	err    error
	asked  []string
}

func (f *fakeImageStore) Get(_ context.Context, key string) (domain.Image, error) {
	f.asked = append(f.asked, key)
	if f.err != nil {
		return domain.Image{}, f.err
	}
	img, ok := f.images[key]
	if !ok {
		return domain.Image{}, errors.New("no such object")
	}
	return img, nil
}

var (
	fallbackImage = domain.Image{Data: []byte("<svg>no image</svg>"), ContentType: "image/svg+xml"}
	redPNG        = domain.Image{Data: []byte("red-bytes"), ContentType: "image/png"}
)

func catalogWithOneImage() *fakeProducts {
	return &fakeProducts{items: []domain.Product{
		{ID: "red", Name: "Red set", Price: 5000, ImageKey: "red.png"},
		{ID: "blue", Name: "Blue set", Price: 3000},
	}}
}

func TestGetProductImage_ServesTheStoredImage(t *testing.T) {
	store := &fakeImageStore{images: map[string]domain.Image{"red.png": redPNG}}
	uc := NewGetProductImage(catalogWithOneImage(), store, fallbackImage)

	got, err := uc.Execute(context.Background(), "red")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != ImageFromStore || string(got.Image.Data) != "red-bytes" || got.Image.ContentType != "image/png" {
		t.Fatalf("got %+v", got)
	}
	if len(store.asked) != 1 || store.asked[0] != "red.png" {
		t.Fatalf("store asked for %v, want [red.png]", store.asked)
	}
}

func TestGetProductImage_ProductWithoutAnImageUsesTheDefaultWithoutAskingTheStore(t *testing.T) {
	store := &fakeImageStore{}
	uc := NewGetProductImage(catalogWithOneImage(), store, fallbackImage)

	got, err := uc.Execute(context.Background(), "blue")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != ImageFallback || string(got.Image.Data) != string(fallbackImage.Data) {
		t.Fatalf("got %+v, want the default image", got)
	}
	if len(store.asked) != 0 {
		t.Fatalf("a product with no image_key must not hit the store, asked %v", store.asked)
	}
}

func TestGetProductImage_AnyStoreFailureFallsBackToTheDefault(t *testing.T) {
	tests := []struct {
		name  string
		store ImageStore
	}{
		{"store unreachable", &fakeImageStore{err: errors.New("dial tcp: connection refused")}},
		{"object missing", &fakeImageStore{images: map[string]domain.Image{}}},
		{"store not configured", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewGetProductImage(catalogWithOneImage(), tt.store, fallbackImage)

			got, err := uc.Execute(context.Background(), "red")

			if err != nil {
				t.Fatalf("a storage problem must never fail the request: %v", err)
			}
			if got.Source != ImageFallback || string(got.Image.Data) != string(fallbackImage.Data) {
				t.Fatalf("got %+v, want the default image", got)
			}
		})
	}
}

func TestGetProductImage_UnknownProductIsAnError(t *testing.T) {
	uc := NewGetProductImage(catalogWithOneImage(), &fakeImageStore{}, fallbackImage)

	if _, err := uc.Execute(context.Background(), "black"); !errors.Is(err, domain.ErrUnknownProduct) {
		t.Fatalf("err = %v, want ErrUnknownProduct", err)
	}
}

func TestGetProductImage_CatalogFailureIsReturned(t *testing.T) {
	boom := errors.New("db down")
	uc := NewGetProductImage(&fakeProducts{err: boom}, &fakeImageStore{}, fallbackImage)

	if _, err := uc.Execute(context.Background(), "red"); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}

func TestGetProductImage_LooksUpOneProductInsteadOfLoadingTheCatalog(t *testing.T) {
	products := catalogWithOneImage()
	uc := NewGetProductImage(products, &fakeImageStore{}, fallbackImage)

	if _, err := uc.Execute(context.Background(), "blue"); err != nil {
		t.Fatal(err)
	}
	if products.listCalls != 0 {
		t.Fatalf("List was called %d times; an image request must not load the whole catalog", products.listCalls)
	}
}

func TestGetProductImage_ReportsWhereThePictureCameFrom(t *testing.T) {
	obs := &recordingObserver{}
	store := &fakeImageStore{images: map[string]domain.Image{"red.png": redPNG}}
	uc := NewGetProductImage(catalogWithOneImage(), store, fallbackImage).WithObserver(obs)
	for _, id := range []string{"red", "blue"} {
		if _, err := uc.Execute(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if len(obs.images) != 2 || !obs.images[0] || obs.images[1] {
		t.Fatalf("images = %v, want [true false]", obs.images)
	}
}
