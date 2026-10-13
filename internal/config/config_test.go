package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFeedsRepositoryConfig(t *testing.T) {
	cfg, err := LoadFeeds(filepath.Join("..", "..", "configs", "feeds.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.GetEnabledFeeds()) == 0 {
		t.Fatal("expected enabled feeds")
	}
	if cfg.Feeds["mirai_tracker"].Enabled {
		t.Fatal("unreachable Mirai feed should remain disabled")
	}
	if cfg.Feeds["urlhaus"].Sources[0].Format != "url" {
		t.Fatal("URLhaus must use URL parsing")
	}
}

func TestLoadFromEnvOverridesAndValidatesProductionAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("environment: development\nserver:\n  port: 8080\ningestor:\n  concurrency: 1\napi:\n  batch_max_size: 10\n  auth_enabled: false\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("PORT", "9091")
	t.Setenv("API_AUTH_ENABLED", "true")
	t.Setenv("API_KEY", "01234567890123456789012345678901")

	cfg, err := LoadFromEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Env != "production" || cfg.Server.Port != 9091 || !cfg.API.AuthEnabled {
		t.Fatalf("environment overrides not applied: %#v", cfg)
	}
}

func TestProductionCannotDisableAuth(t *testing.T) {
	cfg := Config{Env: "production"}
	cfg.Server.Port = 8080
	cfg.Ingestor.Concurrency = 1
	cfg.API.BatchMaxSize = 1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected production authentication validation error")
	}
}

func TestProductionRejectsWildcardCORS(t *testing.T) {
	cfg := Config{Env: "production"}
	cfg.Server.Port = 8080
	cfg.Ingestor.Concurrency = 1
	cfg.API.BatchMaxSize = 1
	cfg.API.AuthEnabled = true
	cfg.API.APIKey = "01234567890123456789012345678901"
	cfg.API.CORS.Enabled = true
	cfg.API.CORS.AllowOrigins = []string{"*"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected wildcard CORS validation error")
	}
}

func TestConnectionURLsOverrideDiscreteFields(t *testing.T) {
	postgres := PostgresConfig{URL: "postgresql://user:pass@example/db", Host: "ignored"}
	if postgres.DSN() != postgres.URL {
		t.Fatalf("DSN() = %q, want URL", postgres.DSN())
	}
}

func TestAdminAndProxyEnvironment(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "admin-012345678901234567890123456789")
	t.Setenv("TRUSTED_PROXIES", "127.0.0.1,10.20.0.1")
	cfg, err := LoadFromEnv(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Server.TrustedProxies) != 2 || cfg.API.AdminKey == "" {
		t.Fatalf("overrides not loaded: %+v", cfg.Server)
	}
	cfg.API.APIKey = cfg.API.AdminKey
	if err := cfg.Validate(); err == nil {
		t.Fatal("admin and lookup key must differ")
	}
	cfg.API.AdminKey = ""
	cfg.Server.TrustedProxies = []string{"0.0.0.0/0"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("must reject trusting all peers")
	}
}

func TestExternalPublisherFeedsRemainVisibleToCompiler(t *testing.T) {
	feeds := &FeedsConfig{Feeds: map[string]FeedConfig{
		"regular": {Enabled: true}, "account": {Enabled: true, ExternallyManaged: true}, "disabled": {Enabled: false},
	}}
	if len(feeds.GetEnabledFeeds()) != 2 || len(feeds.GetIngestibleFeeds()) != 1 {
		t.Fatal("external feed visibility is incorrect")
	}
	if _, ok := feeds.GetEnabledFeeds()["account"]; !ok {
		t.Fatal("compiler lost external source")
	}
}
