package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/comnarin/foodstore/internal/usecase"
)

var _ usecase.LimitRepository = (*LimitRepository)(nil)

func TestLimitRepository_ActiveLimitsReturnsRedForOneHour(t *testing.T) {
	got, err := NewLimitRepository(testDB(t)).ActiveLimits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["red"] != time.Hour {
		t.Fatalf("limits = %v, want map[red:1h]", got)
	}
}
