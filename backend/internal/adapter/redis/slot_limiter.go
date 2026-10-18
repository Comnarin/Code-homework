// Package redis implements usecase.SlotLimiter on Redis.
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
)

type SlotLimiter struct {
	client goredis.Cmdable
}

func NewSlotLimiter(client goredis.Cmdable) *SlotLimiter {
	return &SlotLimiter{client: client}
}

func slotKey(p domain.ProductID) string {
	return "limit:" + string(p) + ":ordered"
}

// claimScript runs as one atomic step in Redis. It returns 1 when it created
// the claim, 0 when the same holder already has it, and -1 when another holder
// does. The stored TTL is never touched for an existing claim.
var claimScript = goredis.NewScript(`
local current = redis.call('GET', KEYS[1])
if not current then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if current == ARGV[1] then
  return 0
end
return -1
`)

// releaseScript deletes the key only if it still holds ARGV[1], so a holder
// can never free a claim that expired and was taken by someone else.
var releaseScript = goredis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

// Claim stores holder under the product's key with an expiry. When many
// requests race, exactly one holder creates the key and the rest are rejected.
// The same holder claiming again succeeds without extending the window.
func (l *SlotLimiter) Claim(ctx context.Context, productID domain.ProductID, holder string, ttl time.Duration) (bool, error) {
	res, err := claimScript.Run(ctx, l.client, []string{slotKey(productID)}, holder, ttl.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("redis claim %s: %w", productID, err)
	}
	switch res {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, domain.ErrSlotHeld
	}
}

// Release deletes the claim only if holder still owns it (compare-and-delete).
func (l *SlotLimiter) Release(ctx context.Context, productID domain.ProductID, holder string) error {
	if err := releaseScript.Run(ctx, l.client, []string{slotKey(productID)}, holder).Err(); err != nil {
		return fmt.Errorf("redis release %s: %w", productID, err)
	}
	return nil
}

// Clear deletes the claim whoever holds it, so the product can be ordered
// again right away (test aid).
func (l *SlotLimiter) Clear(ctx context.Context, productID domain.ProductID) error {
	if err := l.client.Del(ctx, slotKey(productID)).Err(); err != nil {
		return fmt.Errorf("redis clear %s: %w", productID, err)
	}
	return nil
}
