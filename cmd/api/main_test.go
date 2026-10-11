package main

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lfrfrfr/beon-ipquality/internal/config"
)

func TestRoutesSeparateAdminAndKeepOperationalEndpointsAvailable(t *testing.T) {
	cfg := &config.Config{}
	cfg.API.AuthEnabled = true
	cfg.API.APIKey = "lookup-012345678901234567890123456789"
	cfg.API.AdminKey = "admin-012345678901234567890123456789"
	cfg.API.RateLimit = 1
	cfg.API.RateLimitWindow = time.Minute
	cfg.Health.Enabled = true
	cfg.Health.Path = "/health"
	cfg.Metrics.Enabled = true
	cfg.Metrics.Path = "/metrics"
	app := fiber.New(serverConfig(cfg))
	defer app.Shutdown()
	setupMiddleware(app, cfg)
	setupRoutes(app, cfg)
	for _, test := range []struct {
		method, path, key string
		status            int
	}{
		{"GET", "/api/v1/stats", cfg.API.APIKey, 200},
		{"GET", "/api/v1/stats", cfg.API.APIKey, 429},
		{"GET", "/metrics", "", 200},
		{"GET", "/health", "", 503}, // No MMDB: unhealthy, never rate limited.
		{"POST", "/api/v1/reload", cfg.API.APIKey, 401},
		{"GET", "/api/v1/cache/stats", cfg.API.AdminKey, 200},
		{"GET", "/api/v1/stats", cfg.API.AdminKey, 401},
	} {
		req := httptest.NewRequest(test.method, test.path, nil)
		req.Header.Set("X-API-Key", test.key)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != test.status {
			t.Fatalf("%s %s: got %d want %d", test.method, test.path, resp.StatusCode, test.status)
		}
	}
}

func TestAdminRoutesDisabledWithoutKey(t *testing.T) {
	cfg := &config.Config{}
	app := fiber.New(serverConfig(cfg))
	defer app.Shutdown()
	setupRoutes(app, cfg)
	for _, path := range []string{"/api/v1/reload", "/api/v1/cache"} {
		method := "POST"
		if path == "/api/v1/cache" {
			method = "DELETE"
		}
		resp, err := app.Test(httptest.NewRequest(method, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("%s: got %d", path, resp.StatusCode)
		}
	}
}

func TestProxyHeadersOnlyTrustedForConfiguredPeer(t *testing.T) {
	for _, trusted := range []bool{false, true} {
		cfg := &config.Config{}
		// Fiber's in-process test connection uses 0.0.0.0 as its remote peer.
		if trusted {
			cfg.Server.TrustedProxies = []string{"0.0.0.0"}
		}
		app := fiber.New(serverConfig(cfg))
		app.Get("/", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"trusted": c.IsProxyTrusted(), "ip": c.IP()}) })
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Real-IP", "8.8.8.8")
		req.Header.Set("X-Forwarded-For", "1.1.1.1")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Trusted bool
			IP      string
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		app.Shutdown()
		if body.Trusted != trusted || (body.IP == "8.8.8.8") != trusted {
			t.Fatalf("trusted=%v got %+v", trusted, body)
		}
	}
}
