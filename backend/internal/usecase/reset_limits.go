package usecase

import (
	"context"
	"fmt"
)

// ResetLimits frees every limited product (the Red set) so the one-per-hour
// rule can be tested again. It is a test aid and is only wired when enabled.
// It clears each claim whoever holds it.
type ResetLimits struct {
	limits  LimitRepository
	limiter SlotLimiter
}

func NewResetLimits(limits LimitRepository, limiter SlotLimiter) *ResetLimits {
	return &ResetLimits{limits: limits, limiter: limiter}
}

func (u *ResetLimits) Execute(ctx context.Context) error {
	limits, err := u.limits.ActiveLimits(ctx)
	if err != nil {
		return fmt.Errorf("reset limits: %w", err)
	}
	for id := range limits {
		if err := u.limiter.Clear(ctx, id); err != nil {
			return fmt.Errorf("reset limits: %w", err)
		}
	}
	return nil
}
