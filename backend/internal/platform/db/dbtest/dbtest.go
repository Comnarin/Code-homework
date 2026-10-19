// Package dbtest gives integration tests a pristine, migrated Postgres
// database of their own. It is test support only and is never linked into the
// server binary.
package dbtest

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

var safeName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

type prepared struct {
	once sync.Once
	url  string
	err  error
}

var (
	mu    sync.Mutex
	cache = map[string]*prepared{}
)

// Fresh returns a GORM handle on a freshly migrated, empty database named
// "<name>_test<suffix>" (suffix "" gives "<name>_test"), or skips the test when
// DATABASE_URL is not set.
//
// DATABASE_URL only tells the tests which Postgres server to use. They never
// read or write the database it names: they drop and recreate their own
// database once per process, so your data cannot break the tests and the tests
// cannot erase your data. Test packages run in parallel, so each package must
// use its own suffix.
func Fresh(t *testing.T, suffix string) *gorm.DB {
	t.Helper()
	serverURL := os.Getenv("DATABASE_URL")
	if serverURL == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}
	mu.Lock()
	p := cache[suffix]
	if p == nil {
		p = &prepared{}
		cache[suffix] = p
	}
	mu.Unlock()
	p.once.Do(func() { p.url, p.err = prepare(serverURL, suffix) })
	if p.err != nil {
		t.Fatalf("prepare test database: %v", p.err)
	}
	gdb, err := db.NewGorm(p.url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(gdb) })
	if err := gdb.Exec("TRUNCATE orders CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	return gdb
}

// URL returns the connection string of the database Fresh prepared for suffix.
// Call it after Fresh.
func URL(suffix string) string {
	mu.Lock()
	defer mu.Unlock()
	if p := cache[suffix]; p != nil {
		return p.url
	}
	return ""
}

// Recreate drops and recreates "<name>_test<suffix>" empty and unmigrated and
// returns its URL (for migration tests). It skips without DATABASE_URL.
func Recreate(t *testing.T, suffix string) string {
	t.Helper()
	serverURL := os.Getenv("DATABASE_URL")
	if serverURL == "" {
		t.Skip("DATABASE_URL not set; skipping Postgres integration test")
	}
	u, testName, err := testTarget(serverURL, suffix)
	if err != nil {
		t.Fatal(err)
	}
	if err := recreate(serverURL, testName); err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + testName
	return u.String()
}

func testTarget(serverURL, suffix string) (*url.URL, string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if !safeName.MatchString(name) || !safeName.MatchString("x"+suffix) {
		return nil, "", fmt.Errorf("database name %q / suffix %q must be letters, digits or underscores", name, suffix)
	}
	return u, name + "_test" + suffix, nil
}

func recreate(serverURL, testName string) error {
	admin, err := db.NewGorm(serverURL)
	if err != nil {
		return err
	}
	defer db.Close(admin) //nolint:errcheck // closing a throwaway admin connection
	for _, stmt := range []string{
		fmt.Sprintf(`DROP DATABASE IF EXISTS "%s" WITH (FORCE)`, testName),
		fmt.Sprintf(`CREATE DATABASE "%s"`, testName),
	} {
		if err := admin.Exec(stmt).Error; err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

func prepare(serverURL, suffix string) (string, error) {
	u, testName, err := testTarget(serverURL, suffix)
	if err != nil {
		return "", err
	}
	if err := recreate(serverURL, testName); err != nil {
		return "", err
	}
	u.Path = "/" + testName
	if err := db.Migrate(u.String()); err != nil {
		return "", err
	}
	return u.String(), nil
}
