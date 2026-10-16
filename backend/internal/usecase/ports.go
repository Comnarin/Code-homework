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
	// Claim records that productID was just ordered and blocks further orders
	// of it for ttl. If it was already ordered within the window, the returned
	// error matches domain.ErrSlotHeld.
	Claim(ctx context.Context, productID domain.ProductID, ttl time.Duration) error
}

type Clock interface{ Now() time.Time }

type IDGenerator interface{ NewID() string }
