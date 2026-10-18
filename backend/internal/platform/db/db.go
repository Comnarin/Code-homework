// Package db holds Postgres connection and migration helpers.
package db

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/comnarin/foodstore/migrations"
)

// NewGorm opens a GORM handle on Postgres and checks the connection.
func NewGorm(url string) (*gorm.DB, error) {
	gdb, err := gorm.Open(postgres.Open(url), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return gdb, nil
}

// Close releases the connection pool behind gdb.
func Close(gdb *gorm.DB) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// Migrate applies all pending migrations. It is safe to call on every start.
func Migrate(databaseURL string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	dsn := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
	dsn = strings.Replace(dsn, "postgresql://", "pgx5://", 1)
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("init migrate: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
