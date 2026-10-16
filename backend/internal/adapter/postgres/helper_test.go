package postgres

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/comnarin/foodstore/internal/platform/db"
)

var migrateOnce sync.Once

// testPool returns a pool on a migrated database with no orders, or skips the
// test when DATABASE_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}
	var migrateErr error
	migrateOnce.Do(func() { migrateErr = db.Migrate(url) })
	if migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	pool, err := db.NewPool(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE orders CASCADE"); err != nil {
		t.Fatal(err)
	}
	return pool
}
