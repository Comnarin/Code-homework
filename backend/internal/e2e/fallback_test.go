// Package e2e exercises the whole Red-set limit stack (HTTP, use case,
// Postgres, Redis) against real Postgres and an in-memory Redis.
package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	httpapi "github.com/comnarin/foodstore/internal/adapter/http"
	"github.com/comnarin/foodstore/internal/adapter/postgres"
	redisadapter "github.com/comnarin/foodstore/internal/adapter/redis"
	"github.com/comnarin/foodstore/internal/platform/db/dbtest"
	"github.com/comnarin/foodstore/internal/usecase"
)

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

type ids struct{}

func (ids) NewID() string { return uuid.NewString() }

type stack struct {
	mr   *miniredis.Miniredis
	base string
	http *http.Client
}

func newStack(t *testing.T) *stack {
	t.Helper()
	gdb := dbtest.Fresh(t, "_e2e")
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{
		Addr: mr.Addr(), DialTimeout: 250 * time.Millisecond, ReadTimeout: 200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond, PoolTimeout: 300 * time.Millisecond, MaxRetries: -1,
	})
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := redisadapter.NewSlotLimiter(rdb, redisadapter.WithCooldown(100*time.Millisecond))
	orders := postgres.NewOrderRepository(gdb)
	calculate := usecase.NewCalculateOrder(usecase.CalculateOrderDeps{
		Products: postgres.NewProductRepository(gdb),
		Members:  postgres.NewMemberRepository(gdb),
		Rules:    postgres.NewRuleRepository(gdb),
		Limits:   postgres.NewLimitRepository(gdb),
		Orders:   orders,
		Limiter:  limiter,
		Clock:    clock{},
		IDs:      ids{},
	})
	app := httpapi.NewApp(httpapi.Deps{
		Calculator: calculate,
		Ready:      ready(gdb, rdb),
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = app.Listener(ln) }()
	t.Cleanup(func() { _ = app.Shutdown() })

	return &stack{
		mr:   mr,
		base: "http://" + ln.Addr().String(),
		http: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 200}},
	}
}

func ready(gdb *gorm.DB, rdb *goredis.Client) httpapi.ReadyFunc {
	return func(ctx context.Context) (httpapi.Readiness, error) {
		sqlDB, err := gdb.DB()
		if err != nil {
			return httpapi.Readiness{}, err
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return httpapi.Readiness{}, err
		}
		var r httpapi.Readiness
		if err := rdb.Ping(ctx).Err(); err != nil {
			r.Degraded = append(r.Degraded, "redis")
		}
		return r, nil
	}
}

func (s *stack) red(t *testing.T, key string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", s.base+"/api/calculate", strings.NewReader(`{"items":[{"productId":"red","quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client-Key", key)
	resp, err := s.http.Do(req)
	if err != nil {
		t.Error(err) // may run in a goroutine, so no Fatal
		return 0
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func (s *stack) wantRed(t *testing.T, key string, want int) {
	t.Helper()
	if got := s.red(t, key); got != want {
		t.Fatalf("red for %q: status %d, want %d", key, got, want)
	}
}

func TestRedisDown_ExactlyOneOfManyConcurrentBuyersWinsRed(t *testing.T) {
	s := newStack(t)
	s.mr.Close()

	const n = 120
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[int]int{}
	var durations []time.Duration
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			t0 := time.Now()
			code := s.red(t, fmt.Sprintf("race-%d", i))
			mu.Lock()
			counts[code]++
			durations = append(durations, time.Since(t0))
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	if counts[200] != 1 || counts[409] != n-1 || len(counts) != 2 {
		t.Fatalf("status counts = %v, want exactly one 200 and %d 409 and no 5xx", counts, n-1)
	}
	sort.Slice(durations, func(a, b int) bool { return durations[a] < durations[b] })
	if p99 := durations[n*99/100]; p99 > time.Second {
		t.Fatalf("p99 = %v, want < 1s", p99)
	}
}

func TestRedisGoesDownAfterAWin_WinnerStaysWinner(t *testing.T) {
	s := newStack(t)
	s.wantRed(t, "A", 200)
	s.mr.Close()
	s.wantRed(t, "B", 409)
	s.wantRed(t, "A", 200)
}

func TestRedisRecovers_DatabaseClaimIsStillRespectedAndRedisKeyReleased(t *testing.T) {
	s := newStack(t)
	s.mr.Close()
	s.wantRed(t, "A", 200) // decided by the Postgres guard alone

	if err := s.mr.Restart(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(250 * time.Millisecond) // past the breaker cooldown

	s.wantRed(t, "B", 409) // Redis admits B, the Postgres guard rejects it
	if s.mr.Exists("limit:red:ordered") {
		t.Fatal("the Redis claim of the rejected buyer must be released")
	}
	s.wantRed(t, "A", 200)
}

func TestReadyz_IsDegradedNotDownWhenRedisIsDown(t *testing.T) {
	s := newStack(t)
	get := func() (int, string) {
		resp, err := s.http.Get(s.base + "/readyz")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, body := get(); code != 200 || !strings.Contains(body, `"ok"`) {
		t.Fatalf("healthy: %d %s", code, body)
	}
	s.mr.Close()
	if code, body := get(); code != 200 || !strings.Contains(body, `"degraded"`) {
		t.Fatalf("redis down: %d %s, want 200 degraded", code, body)
	}
}
