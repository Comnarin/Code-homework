package httpapi

import (
	"context"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

// The handlers depend on these small interfaces, not on concrete use cases.
type ProductUsecase interface {
	List(ctx context.Context) ([]domain.Product, error)
}

type PromotionUsecase interface {
	List(ctx context.Context) ([]domain.Promotion, error)
}

type ImageGetter interface {
	Execute(ctx context.Context, productID string) (usecase.ImageResult, error)
}

type LimitResetter interface {
	Execute(ctx context.Context) error
}

type OrderCalculator interface {
	Execute(ctx context.Context, in usecase.CalculateInput) (domain.Quote, error)
}

// ReadinessChecker reports whether the service's dependencies (Postgres,
// Redis) can be reached right now.
type ReadinessChecker interface {
	Check(ctx context.Context) error
}

// ReadyFunc adapts a plain function to ReadinessChecker.
type ReadyFunc func(ctx context.Context) error

func (f ReadyFunc) Check(ctx context.Context) error { return f(ctx) }
