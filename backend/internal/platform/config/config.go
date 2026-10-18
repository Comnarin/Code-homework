// Package config reads process configuration from environment variables.
package config

import (
	"errors"
	"io/fs"

	"github.com/joho/godotenv"
)

type Config struct {
	Port        string
	DatabaseURL string
	RedisAddr   string
	// EnableTestTools exposes POST /api/test/reset-limits. Off by default.
	EnableTestTools bool
	// ObjectStorage holds product pictures. Optional: without an endpoint the
	// API serves the default "no image" picture for every product.
	ObjectStorage ObjectStorage
}

type ObjectStorage struct {
	Endpoint  string // host:port, no scheme. Empty disables object storage.
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

func (o ObjectStorage) Enabled() bool { return o.Endpoint != "" }

func Load(getenv func(string) string) (Config, error) {
	c := Config{
		Port:            orDefault(getenv("PORT"), "8080"),
		DatabaseURL:     getenv("DATABASE_URL"),
		RedisAddr:       orDefault(getenv("REDIS_ADDR"), "localhost:6379"),
		EnableTestTools: getenv("ENABLE_TEST_TOOLS") == "true",
		ObjectStorage: ObjectStorage{
			Endpoint:  getenv("MINIO_ENDPOINT"),
			AccessKey: getenv("MINIO_ACCESS_KEY"),
			SecretKey: getenv("MINIO_SECRET_KEY"),
			Bucket:    orDefault(getenv("MINIO_BUCKET"), "product-images"),
			UseSSL:    getenv("MINIO_USE_SSL") == "true",
		},
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if c.ObjectStorage.Enabled() && (c.ObjectStorage.AccessKey == "" || c.ObjectStorage.SecretKey == "") {
		return Config{}, errors.New("MINIO_ACCESS_KEY and MINIO_SECRET_KEY are required when MINIO_ENDPOINT is set")
	}
	return c, nil
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// LoadDotEnv copies variables from a .env file into the process environment.
// Variables that are already set win over the file, so real environment
// variables (Docker, CI, production) always take precedence. A missing file is
// not an error; a malformed one is.
func LoadDotEnv(path string) error {
	err := godotenv.Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
