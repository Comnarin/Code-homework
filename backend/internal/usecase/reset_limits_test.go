package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/comnarin/foodstore/internal/domain"
)

func TestResetLimits_FreesEveryLimitedProduct(t *testing.T) {
	limiter := &fakeLimiter{}
	_, _ = limiter.Claim(context.Background(), "red", "someone", time.Hour)
	uc := NewResetLimits(&fakeLimits{limits: map[domain.ProductID]time.Duration{"red": time.Hour}}, limiter)

	if err := uc.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := limiter.Claim(context.Background(), "red", "another", time.Hour); err != nil {
		t.Fatalf("red should be free after reset: %v", err)
	}
}

func TestResetLimits_PropagatesLimiterErrors(t *testing.T) {
	boom := errors.New("redis down")
	uc := NewResetLimits(&fakeLimits{limits: map[domain.ProductID]time.Duration{"red": time.Hour}}, &fakeLimiter{err: boom})

	if err := uc.Execute(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
}
