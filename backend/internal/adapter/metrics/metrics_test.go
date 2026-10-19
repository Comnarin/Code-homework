package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	goredis "github.com/redis/go-redis/v9"

	"github.com/comnarin/foodstore/internal/domain"
	"github.com/comnarin/foodstore/internal/usecase"
)

func TestObserver_CountsBusinessEvents(t *testing.T) {
	o := NewObserver(prometheus.NewRegistry())
	q := domain.Quote{Discounts: []domain.Discount{{RuleCode: "PAIR_5"}, {RuleCode: "MEMBER_10"}}}
	o.OrderCalculated(q, true)
	o.OrderCalculated(domain.Quote{}, false)
	o.LimitClaim("red", usecase.ClaimSourceDatabase, usecase.ClaimRejected)
	o.LimiterFallback("red")
	o.ImageServed(true)
	o.ImageServed(false)
	o.ImageServed(false)

	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"member orders", testutil.ToFloat64(o.orders.WithLabelValues("true")), 1},
		{"anonymous orders", testutil.ToFloat64(o.orders.WithLabelValues("false")), 1},
		{"pair promo", testutil.ToFloat64(o.promos.WithLabelValues("PAIR_5")), 1},
		{"member promo", testutil.ToFloat64(o.promos.WithLabelValues("MEMBER_10")), 1},
		{"db rejection", testutil.ToFloat64(o.claims.WithLabelValues("red", "db", "rejected")), 1},
		{"fallback", testutil.ToFloat64(o.fallbacks.WithLabelValues("red")), 1},
		{"store image", testutil.ToFloat64(o.images.WithLabelValues("store")), 1},
		{"fallback image", testutil.ToFloat64(o.images.WithLabelValues("fallback")), 2},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestRedisHook_CountsCommandsAndErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	hook := NewRedisHook(prometheus.NewRegistry())
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })
	client.AddHook(hook)
	ctx := context.Background()

	if err := client.Set(ctx, "k", "v", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.Get(ctx, "missing").Err(); err != goredis.Nil {
		t.Fatalf("err = %v", err)
	}
	if got := testutil.ToFloat64(hook.errors.WithLabelValues("get")); got != 0 {
		t.Fatalf("a nil reply counted as an error: %v", got)
	}
	if n := testutil.CollectAndCount(hook.duration); n < 2 {
		t.Fatalf("duration series = %d, want at least set and get", n)
	}

	mr.Close()
	if err := client.Set(ctx, "k", "v", 0).Err(); err == nil {
		t.Fatal("want an error after the server is gone")
	}
	if got := testutil.ToFloat64(hook.errors.WithLabelValues("set")); got != 1 {
		t.Fatalf("set errors = %v, want 1", got)
	}
}

func TestAvailabilityGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	up := true
	RegisterAvailability(reg, "x_available", "h", func() bool { return up })
	if n, err := testutil.GatherAndCount(reg, "x_available"); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
