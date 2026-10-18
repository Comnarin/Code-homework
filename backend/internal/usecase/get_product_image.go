package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/comnarin/foodstore/internal/domain"
)

type ImageSource int

const (
	ImageFromStore ImageSource = iota
	ImageFallback
)

// ImageResult is the picture to serve and where it came from, so callers can
// cache a real image for a while but never a stand-in.
type ImageResult struct {
	Image  domain.Image
	Source ImageSource
}

// GetProductImage returns a product's picture, or the default "no image"
// picture whenever it cannot get a real one. Storage trouble (not configured,
// unreachable, object missing, unservable file) never fails the request.
type GetProductImage struct {
	products ProductRepository
	store    ImageStore // may be nil when object storage is not configured
	fallback domain.Image
}

func NewGetProductImage(products ProductRepository, store ImageStore, fallback domain.Image) *GetProductImage {
	return &GetProductImage{products: products, store: store, fallback: fallback}
}

func (u *GetProductImage) Execute(ctx context.Context, productID string) (ImageResult, error) {
	product, found, err := u.products.FindByID(ctx, domain.ProductID(productID))
	if err != nil {
		return ImageResult{}, fmt.Errorf("find product: %w", err)
	}
	if !found {
		return ImageResult{}, fmt.Errorf("%w: %q", domain.ErrUnknownProduct, productID)
	}

	if product.ImageKey == "" || u.store == nil {
		return u.fallbackResult(), nil
	}
	img, err := u.store.Get(ctx, product.ImageKey)
	if err != nil {
		slog.Warn("product image unavailable, using the default", "product", productID, "key", product.ImageKey, "err", err)
		return u.fallbackResult(), nil
	}
	return ImageResult{Image: img, Source: ImageFromStore}, nil
}

func (u *GetProductImage) fallbackResult() ImageResult {
	return ImageResult{Image: u.fallback, Source: ImageFallback}
}
