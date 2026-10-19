package db_test

import (
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/comnarin/foodstore/internal/platform/db"
	"github.com/comnarin/foodstore/internal/platform/db/dbtest"
	"github.com/comnarin/foodstore/migrations"
)

func TestMigrations_LatestStepCanBeRolledBackAndReapplied(t *testing.T) {
	url := dbtest.Recreate(t, "_migr")
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	gdb, err := db.NewGorm(url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(gdb) //nolint:errcheck // test cleanup

	has := func(table, column string) bool {
		var n int
		if err := gdb.Raw(`SELECT count(*) FROM information_schema.columns
			WHERE table_name = ? AND column_name = ?`, table, column).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n == 1
	}
	if !has("orders", "client_key") || !has("order_items", "started_limit_window") {
		t.Fatal("columns missing after Up")
	}

	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, strings.Replace(url, "postgres://", "pgx5://", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if has("orders", "client_key") || has("order_items", "started_limit_window") {
		t.Fatal("columns still present after the down migration")
	}
	if err := db.Migrate(url); err != nil {
		t.Fatal(err)
	}
	if !has("orders", "client_key") || !has("order_items", "started_limit_window") {
		t.Fatal("columns missing after re-applying Up")
	}
}
