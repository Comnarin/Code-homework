// Package redis implements usecase.SlotLimiter on Redis.
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
)

type SlotLimiter struct{ client goredis.Cmdable }

func NewSlotLimiter(client goredis.Cmdable) *SlotLimiter { return &SlotLimiter{client: client} }

func slotKey(p domain.ProductID) string { return "limit:" + string(p) + ":ordered" }

// Claim uses SET NX with an expiry. Redis runs it as one atomic command, so
// when many requests race, exactly one creates the key and the rest are rejected.
func (l *SlotLimiter) Claim(ctx context.Context, productID domain.ProductID, ttl time.Duration) error {
	won, err := l.client.SetNX(ctx, slotKey(productID), "1", ttl).Result()
	if err != nil {
		return fmt.Errorf("redis claim %s: %w", productID, err)
	}
	if !won {
		return domain.ErrSlotHeld
	}
	return nil
}
