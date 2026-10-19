package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.SlotLimiter = (*SlotLimiter)(nil)

func newLimiter(t *testing.T) (*SlotLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewSlotLimiter(client), mr
}

func TestSlotLimiter_FirstClaimWinsAndStoresTheHolderWithAnExpiry(t *testing.T) {
	l, mr := newLimiter(t)

	acquired, err := l.Claim(context.Background(), "red", "holder-1", time.Hour)
	if err != nil || !acquired {
		t.Fatalf("acquired=%v err=%v, want a fresh claim", acquired, err)
	}
	if got, _ := mr.Get("limit:red:ordered"); got != "holder-1" {
		t.Fatalf("stored value = %q, want the holder", got)
	}
	if ttl := mr.TTL("limit:red:ordered"); ttl != time.Hour {
		t.Fatalf("ttl = %v, want 1h", ttl)
	}
}

func TestSlotLimiter_AnotherHolderInTheWindowIsRejected(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "holder-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(30 * time.Minute)

	if _, err := l.Claim(ctx, "red", "holder-2", time.Hour); !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want ErrSlotHeld", err)
	}
	if ttl := mr.TTL("limit:red:ordered"); ttl != 30*time.Minute {
		t.Fatalf("a rejected claim must not extend the window, ttl = %v", ttl)
	}
}

func TestSlotLimiter_TheHolderCanClaimAgainWithoutExtendingTheWindow(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "holder-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(20 * time.Minute)

	acquired, err := l.Claim(ctx, "red", "holder-1", time.Hour)
	if err != nil {
		t.Fatalf("the holder's own repeat claim must succeed: %v", err)
	}
	if acquired {
		t.Fatal("a repeat claim must report it did not create the claim")
	}
	if ttl := mr.TTL("limit:red:ordered"); ttl != 40*time.Minute {
		t.Fatalf("a repeat claim must not extend the window, ttl = %v", ttl)
	}
}

func TestSlotLimiter_SlotFreesAfterTheWindow(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "holder-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(time.Hour + time.Second)

	if _, err := l.Claim(ctx, "red", "holder-2", time.Hour); err != nil {
		t.Fatalf("slot should be free after the window: %v", err)
	}
}

func TestSlotLimiter_ProductsAreIndependent(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "holder-1", time.Hour); err != nil {
		t.Fatal(err)
	}

	if _, err := l.Claim(ctx, "diamond", "holder-2", time.Hour); err != nil {
		t.Fatalf("a different product has its own window: %v", err)
	}
}

func TestSlotLimiter_ExactlyOneOfManyConcurrentHoldersWins(t *testing.T) {
	l, _ := newLimiter(t)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Claim(context.Background(), "red", fmt.Sprintf("holder-%d", i), time.Hour); err == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("%d holders won the window, want exactly 1", wins.Load())
	}
}

func TestSlotLimiter_RedisErrorIsReturnedNotMistakenForHeld(t *testing.T) {
	l, mr := newLimiter(t)
	mr.Close()

	_, err := l.Claim(context.Background(), "red", "holder-1", time.Hour)
	if err == nil || errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want a connection error that is not ErrSlotHeld", err)
	}
}

func TestSlotLimiter_ReleaseFreesTheSlotForItsHolder(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "holder-1", time.Hour); err != nil {
		t.Fatal(err)
	}

	if err := l.Release(ctx, "red", "holder-1"); err != nil {
		t.Fatal(err)
	}
	if mr.Exists("limit:red:ordered") {
		t.Fatal("release should delete the claim")
	}
	if _, err := l.Claim(ctx, "red", "holder-2", time.Hour); err != nil {
		t.Fatalf("red should be claimable again: %v", err)
	}
}

func TestSlotLimiter_ReleaseNeverFreesSomeoneElsesClaim(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "holder-2", time.Hour); err != nil {
		t.Fatal(err)
	}

	if err := l.Release(ctx, "red", "holder-1"); err != nil {
		t.Fatalf("releasing a claim that is not yours is a no-op, not an error: %v", err)
	}
	if got, _ := mr.Get("limit:red:ordered"); got != "holder-2" {
		t.Fatalf("the other holder's claim was touched, value = %q", got)
	}
}

func TestSlotLimiter_ClearFreesTheSlotWhoeverHoldsIt(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "holder-1", time.Hour); err != nil {
		t.Fatal(err)
	}

	if err := l.Clear(ctx, "red"); err != nil {
		t.Fatal(err)
	}
	if mr.Exists("limit:red:ordered") {
		t.Fatal("clear should delete the claim")
	}
}

func newBreakerLimiter(t *testing.T, now *time.Time) (*SlotLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	l := NewSlotLimiter(client, WithCooldown(5*time.Second), WithOpTimeout(200*time.Millisecond), WithClock(func() time.Time { return *now }))
	return l, mr
}

func TestSlotLimiter_BreakerOpensAfterConnectionFailureAndRecovers(t *testing.T) {
	now := time.Now()
	l, mr := newBreakerLimiter(t, &now)
	ctx := context.Background()
	if _, err := l.Claim(ctx, "red", "a", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.Close()

	_, err := l.Claim(ctx, "red", "a", time.Hour)
	if err == nil || errors.Is(err, domain.ErrSlotHeld) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("first call after outage err = %v, want a raw connection error", err)
	}
	if l.Available() {
		t.Fatal("breaker should be open")
	}
	start := time.Now()
	_, err = l.Claim(ctx, "red", "a", time.Hour)
	if !errors.Is(err, ErrUnavailable) || time.Since(start) > 5*time.Millisecond {
		t.Fatalf("second call err=%v took %v, want instant ErrUnavailable", err, time.Since(start))
	}
	if err := l.Release(ctx, "red", "a"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("release err = %v", err)
	}
	if err := l.Clear(ctx, "red"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("clear err = %v", err)
	}

	now = now.Add(6 * time.Second)
	if err := mr.Restart(); err != nil {
		t.Fatal(err)
	}
	// miniredis keeps its data across Restart, so "a" still holds the claim.
	acquired, err := l.Claim(ctx, "red", "a", time.Hour)
	if err != nil || acquired {
		t.Fatalf("after cooldown + restart: acquired=%v err=%v, want the existing claim reused", acquired, err)
	}
	if !l.Available() {
		t.Fatal("breaker should be closed again")
	}
}

func TestSlotLimiter_ServerErrorsDoNotOpenTheBreaker(t *testing.T) {
	now := time.Now()
	l, mr := newBreakerLimiter(t, &now)
	mr.SetError("boom")
	if _, err := l.Claim(context.Background(), "red", "a", time.Hour); err == nil {
		t.Fatal("want the server error")
	}
	if !l.Available() {
		t.Fatal("a Redis reply is not an outage; breaker must stay closed")
	}
}

func TestSlotLimiter_CancelledCallerDoesNotOpenTheBreaker(t *testing.T) {
	now := time.Now()
	l, _ := newBreakerLimiter(t, &now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Claim(ctx, "red", "a", time.Hour); err == nil {
		t.Fatal("want an error for a cancelled context")
	}
	if !l.Available() {
		t.Fatal("caller cancellation must not open the breaker")
	}
}
