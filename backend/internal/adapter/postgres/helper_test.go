package postgres

import (
	"os"
	"sync"
	"testing"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/platform/db"
)

var migrateOnce sync.Once

// testDB returns a GORM handle on a migrated database with no orders, or skips
// the test when DATABASE_URL is not set.
func testDB(t *testing.T) *gorm.DB {
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
	gdb, err := db.NewGorm(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(gdb) })
	if err := gdb.Exec("TRUNCATE orders CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	return gdb
}
