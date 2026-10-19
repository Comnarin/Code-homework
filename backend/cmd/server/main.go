// Command server wires the layers together and runs the API.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/adapter/assets"
	httpapi "github.com/comnarin/foodstore/internal/adapter/http"
	metricsadapter "github.com/comnarin/foodstore/internal/adapter/metrics"
	miniostore "github.com/comnarin/foodstore/internal/adapter/minio"
	"github.com/comnarin/foodstore/internal/adapter/postgres"
	redisadapter "github.com/comnarin/foodstore/internal/adapter/redis"
	"github.com/comnarin/foodstore/internal/platform/config"
	"github.com/comnarin/foodstore/internal/platform/db"
	platformmetrics "github.com/comnarin/foodstore/internal/platform/metrics"
	"github.com/comnarin/foodstore/internal/platform/telemetry"
	"github.com/comnarin/foodstore/internal/usecase"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type uuidGenerator struct{}

func (uuidGenerator) NewID() string { return uuid.NewString() }

// version is stamped into traces; override with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// JSON logs; records logged with a request context carry trace_id/span_id.
	slog.SetDefault(slog.New(telemetry.NewLogHandler(slog.NewJSONHandler(os.Stderr, nil))))
	if err := run(); err != nil {
		slog.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Local development: read backend/.env if it exists. Real environment
	// variables (Docker, CI, production) always win over the file.
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	if err := db.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := telemetry.Setup(ctx, version)
	if err != nil {
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(flushCtx)
	}()

	gdb, err := db.NewGorm(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close(gdb) //nolint:errcheck // closing at shutdown
	if cfg.OTLPEndpoint != "" {
		// Query arguments are left out of spans: they can hold member cards.
		if err := gdb.Use(telemetry.NewGormTracing()); err != nil {
			return fmt.Errorf("trace database calls: %w", err)
		}
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}

	// Redis is a fast gate in front of the Postgres guard, not a hard
	// dependency: short timeouts, no retries, and a start-up ping failure is
	// only a warning.
	rdb := goredis.NewClient(&goredis.Options{
		Addr:         cfg.RedisAddr,
		DB:           cfg.RedisDB,
		DialTimeout:  250 * time.Millisecond,
		ReadTimeout:  200 * time.Millisecond,
		WriteTimeout: 200 * time.Millisecond,
		PoolTimeout:  300 * time.Millisecond,
		MaxRetries:   -1, // go-redis treats 0 as "default 3"; -1 disables retries
	})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("Redis is not reachable; Red set limit will use the Postgres guard until Redis is reachable", "addr", cfg.RedisAddr, "err", err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewDBStatsCollector(sqlDB, "foodstore"))
	rdb.AddHook(metricsadapter.NewRedisHook(reg))
	metricsadapter.RegisterRedisPool(reg, rdb)
	if cfg.OTLPEndpoint != "" {
		if err := redisotel.InstrumentTracing(rdb); err != nil {
			return fmt.Errorf("trace redis calls: %w", err)
		}
	}

	products := postgres.NewProductRepository(gdb)
	rules := postgres.NewRuleRepository(gdb)
	limits := postgres.NewLimitRepository(gdb)
	orders := postgres.NewOrderRepository(gdb)
	limiter := redisadapter.NewSlotLimiter(rdb)
	metricsadapter.RegisterAvailability(reg, "foodstore_redis_available",
		"1 when the Redis limiter is believed reachable, 0 while its circuit breaker is open.", limiter.Available)
	observer := metricsadapter.NewObserver(reg)
	calculate := usecase.NewCalculateOrder(usecase.CalculateOrderDeps{
		Products: products,
		Members:  postgres.NewMemberRepository(gdb),
		Rules:    rules,
		Limits:   limits,
		Orders:   orders,
		Limiter:  limiter,
		Clock:    systemClock{},
		IDs:      uuidGenerator{},
		Observer: observer,
	})

	imageStore := newImageStore(ctx, cfg)
	if store, ok := imageStore.(*miniostore.Store); ok {
		metricsadapter.RegisterAvailability(reg, "foodstore_object_storage_available",
			"1 when object storage is believed reachable, 0 during its post-failure cooldown.", store.Available)
	}
	deps := httpapi.Deps{
		Products:      usecase.NewProducts(products),
		Promotions:    usecase.NewPromotions(rules),
		Calculator:    calculate,
		ProductImages: usecase.NewGetProductImage(products, imageStore, assets.NoImage()).WithObserver(observer),
		Ready:         readiness(gdb, rdb),
		HTTPMetrics:   httpapi.NewHTTPMetrics(reg),
	}
	if cfg.OTLPEndpoint != "" {
		deps.Tracer = otel.Tracer("github.com/comnarin/foodstore/http")
	}
	if cfg.EnableTestTools {
		slog.Warn("test tools enabled: POST /api/test/reset-limits is exposed")
		deps.ResetLimits = usecase.NewResetLimits(limits, limiter, orders)
	}
	app := httpapi.NewApp(deps)

	// Metrics live on their own port, never on the public router.
	if cfg.MetricsAddr != "" {
		platformmetrics.Serve(ctx, cfg.MetricsAddr, reg)
	}

	go func() {
		<-ctx.Done()
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()
	slog.Info("listening", "port", cfg.Port)
	return app.Listen(":" + cfg.Port)
}

// readiness backs GET /readyz. Postgres must answer a ping or the service is
// not ready. Redis is a soft dependency: when it is down the Postgres guard
// still enforces the Red set limit, so the service reports "degraded" and
// stays in rotation. MinIO is left out on purpose, because pictures fall back
// to the default.
func readiness(gdb *gorm.DB, rdb *goredis.Client) httpapi.ReadyFunc {
	return func(ctx context.Context) (httpapi.Readiness, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		sqlDB, err := gdb.DB()
		if err != nil {
			return httpapi.Readiness{}, err
		}
		if err := sqlDB.PingContext(ctx); err != nil {
			return httpapi.Readiness{}, fmt.Errorf("postgres: %w", err)
		}
		var r httpapi.Readiness
		if err := rdb.Ping(ctx).Err(); err != nil {
			slog.WarnContext(ctx, "redis unreachable; running degraded on the Postgres guard", "err", err)
			r.Degraded = append(r.Degraded, "redis")
		}
		return r, nil
	}
}

// newImageStore connects product pictures to MinIO when it is configured. It
// returns nil when it is not, and the use case then serves the default picture.
// MinIO being down at start-up is only a warning: pictures fall back to the
// default and start working again on their own once MinIO is back.
func newImageStore(ctx context.Context, cfg config.Config) usecase.ImageStore {
	oc := cfg.ObjectStorage
	if !oc.Enabled() {
		slog.Info("object storage not configured: every product shows the default picture")
		return nil
	}
	store, err := miniostore.New(oc.Endpoint, oc.AccessKey, oc.SecretKey, oc.Bucket, oc.UseSSL)
	if err != nil {
		slog.Warn("object storage disabled: invalid configuration", "err", err)
		return nil
	}
	if err := store.EnsureBucket(ctx); err != nil {
		slog.Warn("object storage not reachable yet; pictures will use the default until it is", "endpoint", oc.Endpoint, "err", err)
	}
	return store
}
