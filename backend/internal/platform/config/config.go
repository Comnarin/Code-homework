// Package config reads process configuration from environment variables.
package config

import (
	"errors"
)

type Config struct {
	Port        string
	DatabaseURL string
	RedisAddr   string
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
	return c, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
