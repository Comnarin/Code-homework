// Package config reads process configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
)

type Config struct {
	Port         string
	DatabaseURL  string
	RedisAddr    string
	CookieSecure bool
}

func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Port:        orDefault(getenv("PORT"), "8080"),
		DatabaseURL: getenv("DATABASE_URL"),
		RedisAddr:   orDefault(getenv("REDIS_ADDR"), "localhost:6379"),
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if v := getenv("COOKIE_SECURE"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be true or false: %w", err)
		}
		c.CookieSecure = b
	}
	return c, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
