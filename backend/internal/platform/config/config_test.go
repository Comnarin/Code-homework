package config

import (
	"os"
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoad_AppliesDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.RedisDB != 0 {
		t.Fatalf("redis-db default wrong: %+v", c)
	}
	if c.Port != "8080" || c.RedisAddr != "localhost:6379" || c.DatabaseURL != "postgres://x" {
		t.Fatalf("got %+v", c)
	}
}

func TestLoad_ReadsOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x", "PORT": "9000", "REDIS_ADDR": "redis:6379",
		"REDIS_DB": "1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.RedisDB != 1 {
		t.Fatalf("got %+v", c)
	}
	if c.Port != "9000" || c.RedisAddr != "redis:6379" {
		t.Fatalf("got %+v", c)
	}
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	if _, err := Load(env(nil)); err == nil {
		t.Fatal("want an error when DATABASE_URL is missing")
	}
}

func TestLoad_TestToolsAreOffUnlessExplicitlyEnabled(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			switch k {
			case "DATABASE_URL":
				return "postgres://x"
			case "ENABLE_TEST_TOOLS":
				return v
			}
			return ""
		}
	}
	if c, _ := Load(env("")); c.EnableTestTools {
		t.Fatal("test tools must default to off")
	}
	if c, _ := Load(env("true")); !c.EnableTestTools {
		t.Fatal("ENABLE_TEST_TOOLS=true should enable them")
	}
}

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDotEnv_SetsMissingVariablesButNeverOverridesTheRealEnvironment(t *testing.T) {
	path := writeEnvFile(t, "FOODSTORE_TEST_FROM_FILE=file\nFOODSTORE_TEST_OVERRIDE=file\n")
	t.Setenv("FOODSTORE_TEST_FROM_FILE", "") // registers cleanup; then make it truly unset
	os.Unsetenv("FOODSTORE_TEST_FROM_FILE")
	t.Setenv("FOODSTORE_TEST_OVERRIDE", "real")

	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}

	if got := os.Getenv("FOODSTORE_TEST_FROM_FILE"); got != "file" {
		t.Fatalf("FROM_FILE = %q, want the value from the file", got)
	}
	if got := os.Getenv("FOODSTORE_TEST_OVERRIDE"); got != "real" {
		t.Fatalf("OVERRIDE = %q, the real environment must win over the file", got)
	}
}

func TestLoadDotEnv_MissingFileIsNotAnError(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "no-such.env")); err != nil {
		t.Fatalf("a missing .env is normal (production sets real env vars): %v", err)
	}
}

func TestLoadDotEnv_MalformedFileIsAnError(t *testing.T) {
	path := writeEnvFile(t, "BROKEN=\"never closed\n")

	if err := LoadDotEnv(path); err == nil {
		t.Fatal("a malformed .env should fail loudly, not be half-applied silently")
	}
}

func TestLoad_ObjectStorageIsOffUnlessAnEndpointIsGiven(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.ObjectStorage.Enabled() {
		t.Fatalf("no MINIO_ENDPOINT means no object storage, got %+v", c.ObjectStorage)
	}
	if c.ObjectStorage.Bucket != "product-images" {
		t.Fatalf("default bucket = %q, want product-images", c.ObjectStorage.Bucket)
	}
}

func TestLoad_ReadsObjectStorageSettings(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL":     "postgres://x",
		"MINIO_ENDPOINT":   "minio:9000",
		"MINIO_ACCESS_KEY": "key",
		"MINIO_SECRET_KEY": "secret",
		"MINIO_BUCKET":     "pics",
		"MINIO_USE_SSL":    "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := c.ObjectStorage
	if !s.Enabled() || s.Endpoint != "minio:9000" || s.AccessKey != "key" || s.SecretKey != "secret" || s.Bucket != "pics" || !s.UseSSL {
		t.Fatalf("got %+v", s)
	}
}

func TestLoad_ObjectStorageNeedsCredentialsWhenEnabled(t *testing.T) {
	_, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "MINIO_ENDPOINT": "minio:9000"}))
	if err == nil {
		t.Fatal("an endpoint without MINIO_ACCESS_KEY / MINIO_SECRET_KEY should be rejected at start-up")
	}
}

func TestLoad_RejectsABadRedisDB(t *testing.T) {
	for _, v := range []string{"-1", "16", "abc"} {
		if _, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x", "REDIS_DB": v})); err == nil {
			t.Fatalf("REDIS_DB=%q should be rejected", v)
		}
	}
}
