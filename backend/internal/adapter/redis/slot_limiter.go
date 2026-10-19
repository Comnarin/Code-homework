// Package redis implements usecase.SlotLimiter on Redis.
package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
)

// ErrUnavailable is returned instantly while the breaker is open, i.e. after a
// recent connection failure. The use case then relies on the Postgres guard.
var ErrUnavailable = errors.New("redis is temporarily unavailable")

const (
	defaultOpTimeout = 200 * time.Millisecond
	defaultCooldown  = 5 * time.Second
)

// SlotLimiter is the Redis admission gate for limited products. After a
// connection failure it answers ErrUnavailable immediately for a cooldown
// instead of making every request wait for a timeout again; the first call
// after the cooldown is the trial.
type SlotLimiter struct {
	client    goredis.Cmdable
	opTimeout time.Duration
	cooldown  time.Duration
	now       func() time.Time

	mu        sync.Mutex
	downUntil time.Time
}

// Option customises a SlotLimiter.
type Option func(*SlotLimiter)

func WithCooldown(d time.Duration) Option   { return func(l *SlotLimiter) { l.cooldown = d } }
func WithOpTimeout(d time.Duration) Option  { return func(l *SlotLimiter) { l.opTimeout = d } }
func WithClock(now func() time.Time) Option { return func(l *SlotLimiter) { l.now = now } }

func NewSlotLimiter(client goredis.Cmdable, opts ...Option) *SlotLimiter {
	l := &SlotLimiter{client: client, opTimeout: defaultOpTimeout, cooldown: defaultCooldown, now: time.Now}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Available reports whether the breaker is closed (Redis is believed to be up).
func (l *SlotLimiter) Available() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.now().Before(l.downUntil)
}

// noteFailure opens the breaker when the failure means "cannot reach Redis"
// (timeout, refused, reset). A reply from the server (including NOSCRIPT) is
// not an outage, and neither is the caller giving up.
func (l *SlotLimiter) noteFailure(callerCtx context.Context, err error) {
	if callerCtx.Err() != nil {
		return
	}
	var serverErr goredis.Error
	if errors.As(err, &serverErr) {
		return
	}
	l.mu.Lock()
	l.downUntil = l.now().Add(l.cooldown)
	l.mu.Unlock()
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
	if !l.Available() {
		return false, fmt.Errorf("%w", ErrUnavailable)
	}
	callerCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, l.opTimeout)
	defer cancel()
	res, err := claimScript.Run(ctx, l.client, []string{slotKey(productID)}, holder, ttl.Milliseconds()).Int()
	if err != nil {
		l.noteFailure(callerCtx, err)
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
	if !l.Available() {
		return fmt.Errorf("%w", ErrUnavailable)
	}
	callerCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, l.opTimeout)
	defer cancel()
	if err := releaseScript.Run(ctx, l.client, []string{slotKey(productID)}, holder).Err(); err != nil {
		l.noteFailure(callerCtx, err)
		return fmt.Errorf("redis release %s: %w", productID, err)
	}
	return nil
}

// Clear deletes the claim whoever holds it, so the product can be ordered
// again right away (test aid).
func (l *SlotLimiter) Clear(ctx context.Context, productID domain.ProductID) error {
	if !l.Available() {
		return fmt.Errorf("%w", ErrUnavailable)
	}
	callerCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, l.opTimeout)
	defer cancel()
	if err := l.client.Del(ctx, slotKey(productID)).Err(); err != nil {
		l.noteFailure(callerCtx, err)
		return fmt.Errorf("redis clear %s: %w", productID, err)
	}
	return nil
}
