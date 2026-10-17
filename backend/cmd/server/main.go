// Command server wires the layers together and runs the API.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	httpapi "github.com/comnarin/foodstore/internal/adapter/http"
	"github.com/comnarin/foodstore/internal/adapter/postgres"
	redisadapter "github.com/comnarin/foodstore/internal/adapter/redis"
	"github.com/comnarin/foodstore/internal/platform/config"
	"github.com/comnarin/foodstore/internal/platform/db"
	"github.com/comnarin/foodstore/internal/usecase"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type uuidGenerator struct{}

func (uuidGenerator) NewID() string { return uuid.NewString() }

func main() {
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

	gdb, err := db.NewGorm(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close(gdb) //nolint:errcheck // closing at shutdown

	rdb := goredis.NewClient(&goredis.Options{Addr: cfg.RedisAddr})
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return err
	}

	products := postgres.NewProductRepository(gdb)
	rules := postgres.NewRuleRepository(gdb)
	limits := postgres.NewLimitRepository(gdb)
	limiter := redisadapter.NewSlotLimiter(rdb)
	calculate := usecase.NewCalculateOrder(usecase.CalculateOrderDeps{
		Products: products,
		Members:  postgres.NewMemberRepository(gdb),
		Rules:    rules,
		Limits:   limits,
		Orders:   postgres.NewOrderRepository(gdb),
		Limiter:  limiter,
		Clock:    systemClock{},
		IDs:      uuidGenerator{},
	})

	deps := httpapi.Deps{
		Products:   usecase.NewListProducts(products),
		Promotions: usecase.NewListPromotions(rules),
		Calculator: calculate,
	}
	if cfg.EnableTestTools {
		slog.Warn("test tools enabled: POST /api/test/reset-limits is exposed")
		deps.ResetLimits = usecase.NewResetLimits(limits, limiter)
	}
	app := httpapi.NewApp(deps)

	go func() {
		<-ctx.Done()
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()
	slog.Info("listening", "port", cfg.Port)
	return app.Listen(":" + cfg.Port)
}
