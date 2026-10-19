package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

func redLimits() *fakeLimits {
	return &fakeLimits{limits: map[domain.ProductID]time.Duration{"red": time.Hour}}
}

func TestResetLimits_FreesEveryLimitedProductInBothStores(t *testing.T) {
	limiter := &fakeLimiter{}
	_, _ = limiter.Claim(context.Background(), "red", "someone", time.Hour)
	windows := &fakeWindows{}
	uc := NewResetLimits(redLimits(), limiter, windows)

	if err := uc.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if windows.cleared != 1 {
		t.Fatal("the Postgres windows must be cleared")
	}
	if _, err := limiter.Claim(context.Background(), "red", "another", time.Hour); err != nil {
		t.Fatalf("red should be free after reset: %v", err)
	}
}

func TestResetLimits_ClearsPostgresEvenWhenRedisIsDown(t *testing.T) {
	windows := &fakeWindows{}
	uc := NewResetLimits(redLimits(), &fakeLimiter{err: errors.New("redis down")}, windows)

	if err := uc.Execute(context.Background()); err != nil {
		t.Fatalf("a Redis outage must not fail the reset: %v", err)
	}
	if windows.cleared != 1 {
		t.Fatal("the Postgres windows must still be cleared")
	}
}

func TestResetLimits_FailsWhenPostgresFails(t *testing.T) {
	boom := errors.New("db down")
	limiter := &fakeLimiter{}
	_, _ = limiter.Claim(context.Background(), "red", "someone", time.Hour)
	uc := NewResetLimits(redLimits(), limiter, &fakeWindows{err: boom})

	if err := uc.Execute(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if limiter.held["red"] != "someone" {
		t.Fatal("Redis must not be cleared when the authoritative clear failed")
	}
}
