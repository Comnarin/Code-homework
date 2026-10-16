package redis

import (
	"context"
	"errors"
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

func TestSlotLimiter_FirstClaimWinsAndExpiresAfterTheWindow(t *testing.T) {
	l, mr := newLimiter(t)

	if err := l.Claim(context.Background(), "red", time.Hour); err != nil {
		t.Fatal(err)
	}
	if !mr.Exists("limit:red:ordered") {
		t.Fatal("the claim should be stored under limit:red:ordered")
	}
	if ttl := mr.TTL("limit:red:ordered"); ttl != time.Hour {
		t.Fatalf("ttl = %v, want 1h", ttl)
	}
}

func TestSlotLimiter_SecondClaimInTheWindowIsRejected(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if err := l.Claim(ctx, "red", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(30 * time.Minute)

	if err := l.Claim(ctx, "red", time.Hour); !errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want ErrSlotHeld", err)
	}
	if ttl := mr.TTL("limit:red:ordered"); ttl != 30*time.Minute {
		t.Fatalf("a rejected claim must not extend the window, ttl = %v", ttl)
	}
}

func TestSlotLimiter_SlotFreesAfterTheWindow(t *testing.T) {
	l, mr := newLimiter(t)
	ctx := context.Background()
	if err := l.Claim(ctx, "red", time.Hour); err != nil {
		t.Fatal(err)
	}
	mr.FastForward(time.Hour + time.Second)

	if err := l.Claim(ctx, "red", time.Hour); err != nil {
		t.Fatalf("slot should be free after the window: %v", err)
	}
}

func TestSlotLimiter_ProductsAreIndependent(t *testing.T) {
	l, _ := newLimiter(t)
	ctx := context.Background()
	if err := l.Claim(ctx, "red", time.Hour); err != nil {
		t.Fatal(err)
	}

	if err := l.Claim(ctx, "diamond", time.Hour); err != nil {
		t.Fatalf("a different product has its own window: %v", err)
	}
}

func TestSlotLimiter_ExactlyOneOfManyConcurrentClaimsWins(t *testing.T) {
	l, _ := newLimiter(t)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Claim(context.Background(), "red", time.Hour); err == nil {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()

	if wins.Load() != 1 {
		t.Fatalf("%d claims won the window, want exactly 1", wins.Load())
	}
}

func TestSlotLimiter_RedisErrorIsReturnedNotMistakenForHeld(t *testing.T) {
	l, mr := newLimiter(t)
	mr.Close()

	err := l.Claim(context.Background(), "red", time.Hour)
	if err == nil || errors.Is(err, domain.ErrSlotHeld) {
		t.Fatalf("err = %v, want a connection error that is not ErrSlotHeld", err)
	}
}
