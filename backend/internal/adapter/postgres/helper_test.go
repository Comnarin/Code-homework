package postgres

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/platform/db"
)

var (
	prepareOnce sync.Once
	testURL     string
	prepareErr  error
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// testDB returns a GORM handle on a pristine, freshly migrated database that
// exists only for the tests, or skips when DATABASE_URL is not set.
//
// DATABASE_URL only tells the tests which Postgres server to use. They never
// read or write the database it names: they drop and recreate "<name>_test" on
// that server once per run, so your own data (and any experiments you make in
// a SQL client) cannot break the tests, and the tests cannot erase your data.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	serverURL := os.Getenv("DATABASE_URL")
	if serverURL == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}
	prepareOnce.Do(func() { testURL, prepareErr = prepareTestDatabase(serverURL) })
	if prepareErr != nil {
		t.Fatalf("prepare test database: %v", prepareErr)
	}
	gdb, err := db.NewGorm(testURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(gdb) })
	if err := gdb.Exec("TRUNCATE orders CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	return gdb
}

func prepareTestDatabase(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !safeName.MatchString(name) {
		return "", fmt.Errorf("database name %q must be letters, digits or underscores", name)
	}
	testName := name + "_test"

	admin, err := db.NewGorm(serverURL)
	if err != nil {
		return "", err
	}
	defer db.Close(admin) //nolint:errcheck // closing a throwaway admin connection
	for _, stmt := range []string{
		fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, testName),
		fmt.Sprintf(`CREATE DATABASE "%s"`, testName),
	} {
		if err := admin.Exec(stmt).Error; err != nil {
			return "", fmt.Errorf("%s: %w", stmt, err)
		}
	}

	u.Path = "/" + testName
	if err := db.Migrate(u.String()); err != nil {
		return "", err
	}
	return u.String(), nil
}
