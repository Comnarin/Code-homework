// Package redis implements usecase.SlotLimiter on Redis.
package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
)

// claimScript is atomic: a check-then-set done in Go would let two customers
// both see an empty slot. Returns 1 when ARGV[1] now holds the slot, else 0.
var claimScript = goredis.NewScript(`
local holder = redis.call('GET', KEYS[1])
if not holder then
  redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
  return 1
end
if holder == ARGV[1] then
  return 1
end
return 0
`)

type SlotLimiter struct{ client goredis.Scripter }

func NewSlotLimiter(client goredis.Scripter) *SlotLimiter { return &SlotLimiter{client: client} }

func slotKey(p domain.ProductID) string { return "limit:" + string(p) + ":holder" }

func (l *SlotLimiter) Claim(ctx context.Context, productID domain.ProductID, ownerID string, ttl time.Duration) error {
	won, err := claimScript.Run(ctx, l.client, []string{slotKey(productID)}, ownerID, ttl.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("redis claim %s: %w", productID, err)
	}
	if won != 1 {
		return domain.ErrSlotHeld
	}
	return nil
}
