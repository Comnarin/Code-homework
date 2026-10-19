package postgres

import (
	"testing"

	"gorm.io/gorm"

	"github.com/comnarin/foodstore/internal/platform/db/dbtest"
)

// testDB returns a GORM handle on a pristine, freshly migrated database that
// exists only for this package's tests, or skips when DATABASE_URL is not set.
// See dbtest.Fresh for how the database is isolated from your own data.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	return dbtest.Fresh(t, "")
}
