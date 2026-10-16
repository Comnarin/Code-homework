package config

import "testing"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoad_AppliesDefaults(t *testing.T) {
	c, err := Load(env(map[string]string{"DATABASE_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "8080" || c.RedisAddr != "localhost:6379" || c.CookieSecure || c.DatabaseURL != "postgres://x" {
		t.Fatalf("got %+v", c)
	}
}

func TestLoad_ReadsOverrides(t *testing.T) {
	c, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://x", "PORT": "9000", "REDIS_ADDR": "redis:6379", "COOKIE_SECURE": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Port != "9000" || c.RedisAddr != "redis:6379" || !c.CookieSecure {
		t.Fatalf("got %+v", c)
	}
}

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	if _, err := Load(env(nil)); err == nil {
		t.Fatal("want an error when DATABASE_URL is missing")
	}
}

func TestLoad_RejectsBadBool(t *testing.T) {
	if _, err := Load(env(map[string]string{"DATABASE_URL": "x", "COOKIE_SECURE": "maybe"})); err == nil {
		t.Fatal("want an error for an unparsable COOKIE_SECURE")
	}
}
