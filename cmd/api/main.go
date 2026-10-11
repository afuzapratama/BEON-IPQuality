package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/lfrfrfr/beon-ipquality/internal/api/handlers"
	"github.com/lfrfrfr/beon-ipquality/internal/api/middleware"
	"github.com/lfrfrfr/beon-ipquality/internal/cache"
	"github.com/lfrfrfr/beon-ipquality/internal/config"
	internalmetrics "github.com/lfrfrfr/beon-ipquality/internal/metrics"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	pkglogger "github.com/lfrfrfr/beon-ipquality/pkg/logger"
)

var (
	version   = "1.0.0"
	buildTime = "unknown"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadFromEnv(*configPath)
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := pkglogger.Init(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output, cfg.Logging.FilePath); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer pkglogger.Sync()

	pkglogger.Info("Starting BEON-IPQuality API Server") // zap.String("version", version),
	// zap.String("environment", cfg.Env),

	// Initialize MMDB reader
	mmdbPath := cfg.MMDB.ReputationPath
	if mmdbPath == "" {
		mmdbPath = "./data/mmdb/reputation.mmdb"
	}
	handlers.SetMMDBConfig(handlers.MMDBConfig{
		ReputationPath: mmdbPath,
		GeoIPCityPath:  cfg.MMDB.GeoLite2CityPath,
		GeoIPASNPath:   cfg.MMDB.GeoLite2ASNPath,
	})

	mmdbReader, err := mmdb.NewReader(mmdbPath, cfg.MMDB.GeoLite2CityPath, cfg.MMDB.GeoLite2ASNPath)
	if err != nil {
		if cfg.Env == "production" {
			pkglogger.Fatal(fmt.Sprintf("Failed to load required MMDB: %v", err))
		}
		pkglogger.Warn(fmt.Sprintf("Failed to load MMDB: %v (reputation endpoints will return 503)", err))
	} else {
		pkglogger.Info(fmt.Sprintf("Loaded MMDB from %s", mmdbPath))
		handlers.SetMMDBReader(mmdbReader)
		reloadCancel, reloadDone := startMMDBReload(mmdbReader, cfg)
		defer func() {
			reloadCancel()
			<-reloadDone
			_ = mmdbReader.Close()
		}()
	}

	// Initialize Redis cache (if enabled)
	if cfg.Redis.Enabled {
		redisCache, err := cache.NewRedisCache(cache.Config{
			URL:      cfg.Redis.URL,
			Host:     cfg.Redis.Host,
			Port:     cfg.Redis.Port,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
			PoolSize: cfg.Redis.PoolSize,
			TTL:      5 * time.Minute,
			Prefix:   "ipq:",
		})
		if err != nil {
			pkglogger.Warn(fmt.Sprintf("Failed to connect to Redis: %v (caching disabled)", err))
		} else {
			pkglogger.Info("Redis cache initialized")
			handlers.SetCache(redisCache)
			defer redisCache.Close()
		}
	} else {
		pkglogger.Info("Redis caching is disabled")
	}

	// Create Fiber app
	app := fiber.New(serverConfig(cfg))

	// Setup middleware
	setupMiddleware(app, cfg)

	// Setup routes
	setupRoutes(app, cfg)

	// Start server
	go func() {
		addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
		pkglogger.Info(fmt.Sprintf("API Server listening on %s", addr))
		if err := app.Listen(addr); err != nil {
			pkglogger.Fatal(fmt.Sprintf("Server failed to start: %v", err))
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	pkglogger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		pkglogger.Error(fmt.Sprintf("Server forced to shutdown: %v", err))
	}

	pkglogger.Info("Server exited gracefully")
}

func startMMDBReload(reader *mmdb.Reader, cfg *config.Config) (context.CancelFunc, <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if cfg.MMDB.ReloadInterval <= 0 {
			<-ctx.Done()
			return
		}
		ticker := time.NewTicker(cfg.MMDB.ReloadInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := reader.Reload(cfg.MMDB.ReputationPath, cfg.MMDB.GeoLite2CityPath, cfg.MMDB.GeoLite2ASNPath); err != nil {
					pkglogger.Error(fmt.Sprintf("Scheduled MMDB reload failed: %v", err))
				} else if cache := handlers.GetConfiguredCache(); cache != nil {
					_ = cache.Clear(context.Background())
				}
			}
		}
	}()
	return cancel, done
}

func setupMiddleware(app *fiber.App, cfg *config.Config) {
	// Recovery middleware
	app.Use(recover.New())

	// Logger middleware
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${method} ${path} (${latency})\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))
	if cfg.Metrics.Enabled {
		app.Use(internalmetrics.HTTPMiddleware())
	}

	// CORS middleware
	if cfg.API.CORS.Enabled {
		app.Use(cors.New(cors.Config{
			AllowOrigins: joinStrings(cfg.API.CORS.AllowOrigins),
			AllowMethods: joinStrings(cfg.API.CORS.AllowMethods),
			AllowHeaders: joinStrings(cfg.API.CORS.AllowHeaders),
		}))
	}
}

func setupRoutes(app *fiber.App, cfg *config.Config) {
	// Health check endpoint (no auth required)
	if cfg.Health.Enabled {
		app.Get(cfg.Health.Path, handlers.HealthCheck(version))
	}

	// Prometheus metrics endpoint (no auth required)
	if cfg.Metrics.Enabled {
		app.Get(cfg.Metrics.Path, adaptor.HTTPHandler(promhttp.Handler()))
	}

	// Admin endpoints stay disabled until a separate key is configured.
	adminAuth := middleware.AdminKeyAuth(cfg.API.AdminKey)
	app.Get("/api/v1/cache/stats", adminAuth, handlers.GetCacheStats())
	app.Delete("/api/v1/cache", adminAuth, handlers.ClearCache())
	app.Post("/api/v1/reload", adminAuth, handlers.ReloadMMDB())

	// API v1 routes
	v1 := app.Group("/api/v1")

	// Apply API key authentication if enabled
	if cfg.API.AuthEnabled {
		v1.Use(middleware.APIKeyAuth(cfg.API.APIKey))
	}

	// Rate limiter middleware
	if cfg.API.RateLimit > 0 {
		v1.Use(limiter.New(limiter.Config{
			Max:        cfg.API.RateLimit,
			Expiration: cfg.API.RateLimitWindow,
			KeyGenerator: func(c *fiber.Ctx) string {
				if cfg.API.AuthEnabled {
					return fmt.Sprintf("key:%x", sha256.Sum256([]byte(c.Get("X-API-Key"))))
				}
				return "ip:" + c.IP()
			},
			LimitReached: func(c *fiber.Ctx) error {
				internalmetrics.APIRateLimitHits.Inc()
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error":   "rate_limit_exceeded",
					"message": "Too many requests. Please try again later.",
				})
			},
		}))
	}

	// IP check endpoints
	v1.Get("/check/:ip", handlers.CheckIP())

	if cfg.API.BatchEnabled {
		v1.Post("/check/batch", handlers.BatchCheckIP(cfg.API.BatchMaxSize))
	}

	// Stats endpoint
	v1.Get("/stats", handlers.GetStats())

	// 404 handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":   "not_found",
			"message": "The requested endpoint does not exist",
		})
	})
}

func joinStrings(s []string) string {
	result := ""
	for i, str := range s {
		if i > 0 {
			result += ", "
		}
		result += str
	}
	return result
}

func serverConfig(cfg *config.Config) fiber.Config {
	return fiber.Config{
		ReadTimeout: cfg.Server.ReadTimeout, WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout: cfg.Server.IdleTimeout, AppName: "BEON-IPQuality API v" + version,
		DisableStartupMessage: cfg.Env == "production",
		ProxyHeader:           "X-Real-IP", EnableTrustedProxyCheck: true,
		TrustedProxies: cfg.Server.TrustedProxies, EnableIPValidation: true,
	}
}
