package usecase

import (
	"context"
	"fmt"
	"log/slog"
)

// ResetLimits frees every limited product (the Red set) so the one-per-hour
// rule can be tested again. It is a test aid and is only wired when enabled.
// It clears each claim whoever holds it: first the Postgres windows (the
// authority, must succeed), then the Redis gate (best effort when Redis is
// unavailable, because an outage already means Postgres decides).
type ResetLimits struct {
	limits  LimitRepository
	limiter SlotLimiter
	windows LimitWindowClearer
}

func NewResetLimits(limits LimitRepository, limiter SlotLimiter, windows LimitWindowClearer) *ResetLimits {
	return &ResetLimits{limits: limits, limiter: limiter, windows: windows}
}

func (u *ResetLimits) Execute(ctx context.Context) error {
	limits, err := u.limits.ActiveLimits(ctx)
	if err != nil {
		return fmt.Errorf("reset limits: %w", err)
	}
	if err := u.windows.ClearLimitWindows(ctx); err != nil {
		return fmt.Errorf("reset limits: %w", err)
	}
	for id := range limits {
		if err := u.limiter.Clear(ctx, id); err != nil {
			slog.WarnContext(ctx, "reset limits: Redis not cleared; the Postgres windows were", "product", id, "err", err)
			return nil
		}
	}
	return nil
}
