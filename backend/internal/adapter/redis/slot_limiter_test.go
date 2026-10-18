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
