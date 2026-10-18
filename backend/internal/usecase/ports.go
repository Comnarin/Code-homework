package usecase

import (
	"context"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

// Ports: what the use cases need from the outside world. Adapters implement them.

type ProductRepository interface {
	List(ctx context.Context) ([]domain.Product, error)
	// FindByID returns false (and no error) when there is no such product.
	FindByID(ctx context.Context, id domain.ProductID) (domain.Product, bool, error)
}

type MemberRepository interface {
	// FindByCard expects a normalized card number.
	FindByCard(ctx context.Context, card string) (domain.Member, bool, error)
}

type RuleRepository interface {
	ActiveRules(ctx context.Context) ([]domain.Rule, error)
}

type LimitRepository interface {
	// ActiveLimits maps each limited product to the length of its exclusive window.
	ActiveLimits(ctx context.Context) (map[domain.ProductID]time.Duration, error)
}

type OrderRepository interface {
	Save(ctx context.Context, o domain.Order) error
}

// SlotLimiter keeps a limited product from being ordered again within its
// window. A claim is owned by a holder, so the holder's own repeat order is not
// blocked while everyone else is.
type SlotLimiter interface {
	// Claim records holder as the one who ordered productID and blocks other
	// holders for ttl. acquired is true when this call created the claim and
	// false when holder already had it (the window is not extended). If another
	// holder has it, the error matches domain.ErrSlotHeld.
	Claim(ctx context.Context, productID domain.ProductID, holder string, ttl time.Duration) (acquired bool, err error)
	// Release frees productID only if holder still owns the claim, so it can
	// never free a claim that expired and was taken by someone else.
	Release(ctx context.Context, productID domain.ProductID, holder string) error
	// Clear frees productID whoever holds it (test aid).
	Clear(ctx context.Context, productID domain.ProductID) error
}

// ImageStore reads product pictures from object storage. Get must return an
// image that already passed domain.NewImage.
type ImageStore interface {
	Get(ctx context.Context, key string) (domain.Image, error)
}

type Clock interface{ Now() time.Time }

type IDGenerator interface{ NewID() string }
