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
	calculate := usecase.NewCalculateOrder(usecase.CalculateOrderDeps{
		Products: products,
		Members:  postgres.NewMemberRepository(gdb),
		Rules:    postgres.NewRuleRepository(gdb),
		Limits:   postgres.NewLimitRepository(gdb),
		Orders:   postgres.NewOrderRepository(gdb),
		Limiter:  redisadapter.NewSlotLimiter(rdb),
		Clock:    systemClock{},
		IDs:      uuidGenerator{},
	})

	app := httpapi.NewApp(httpapi.Deps{
		Products:   usecase.NewListProducts(products),
		Calculator: calculate,
	})

	go func() {
		<-ctx.Done()
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()
	slog.Info("listening", "port", cfg.Port)
	return app.Listen(":" + cfg.Port)
}
