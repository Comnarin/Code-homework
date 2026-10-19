package metrics

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	goredis "github.com/redis/go-redis/v9"
)

// RedisHook times every Redis command and counts the ones that fail.
// Redis "nil" replies (a missing key) are normal answers, not errors.
type RedisHook struct {
	duration *prometheus.HistogramVec
	errors   *prometheus.CounterVec
}

var _ goredis.Hook = (*RedisHook)(nil)

func NewRedisHook(reg prometheus.Registerer) *RedisHook {
	h := &RedisHook{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "redis_command_duration_seconds",
			Help:    "Redis command latency, by command.",
			Buckets: []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5},
		}, []string{"command"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "redis_command_errors_total", Help: "Redis commands that failed, by command.",
		}, []string{"command"}),
	}
	reg.MustRegister(h.duration, h.errors)
	return h
}

func (h *RedisHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := next(ctx, network, addr)
		if err != nil {
			h.errors.WithLabelValues("dial").Inc()
		}
		return conn, err
	}
}

func (h *RedisHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmd)
		h.record(cmd.Name(), start, err)
		return err
	}
}

func (h *RedisHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		start := time.Now()
		err := next(ctx, cmds)
		h.record("pipeline", start, err)
		return err
	}
}

func (h *RedisHook) record(command string, start time.Time, err error) {
	h.duration.WithLabelValues(command).Observe(time.Since(start).Seconds())
	if err != nil && !errors.Is(err, goredis.Nil) {
		h.errors.WithLabelValues(command).Inc()
	}
}

// RegisterRedisPool exposes connection pool gauges for client.
func RegisterRedisPool(reg prometheus.Registerer, client *goredis.Client) {
	reg.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "redis_pool_total_conns", Help: "Connections currently in the Redis pool.",
		}, func() float64 { return float64(client.PoolStats().TotalConns) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "redis_pool_timeouts_total", Help: "Times a request waited too long for a Redis connection.",
		}, func() float64 { return float64(client.PoolStats().Timeouts) }),
	)
}

// RegisterAvailability exposes a 1/0 gauge for a dependency that reports
// whether it is believed to be reachable.
func RegisterAvailability(reg prometheus.Registerer, name, help string, available func() bool) {
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help}, func() float64 {
		if available() {
			return 1
		}
		return 0
	}))
}
