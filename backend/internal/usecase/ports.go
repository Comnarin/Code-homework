package usecase

import (
	"context"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

// Ports: what the use cases need from the outside world. Adapters implement them.

type ProductRepository interface {
	List(ctx context.Context) ([]domain.Product, error)
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

type SlotLimiter interface {
	// Claim reserves the slot for ownerID. It succeeds if the slot is free or
	// already held by ownerID (without extending it). If another owner holds
	// it, the returned error matches domain.ErrSlotHeld.
	Claim(ctx context.Context, productID domain.ProductID, ownerID string, ttl time.Duration) error
}

type Clock interface{ Now() time.Time }

type IDGenerator interface{ NewID() string }
