package main

import (
	"context"
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
	cfg, err := config.Load(*configPath)
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

	mmdbReader, err := mmdb.NewReader(mmdbPath, cfg.MMDB.GeoLite2CityPath, cfg.MMDB.GeoLite2ASNPath)
	if err != nil {
		pkglogger.Warn(fmt.Sprintf("Failed to load MMDB: %v (API will return clean results)", err))
	} else {
		pkglogger.Info(fmt.Sprintf("Loaded MMDB from %s", mmdbPath))
		handlers.SetMMDBReader(mmdbReader)
		// Set MMDB config for hot reload
		handlers.SetMMDBConfig(handlers.MMDBConfig{
			ReputationPath: mmdbPath,
			GeoIPCityPath:  cfg.MMDB.GeoLite2CityPath,
			GeoIPASNPath:   cfg.MMDB.GeoLite2ASNPath,
		})
		defer mmdbReader.Close()
	}

	// Initialize Redis cache (if enabled)
	if cfg.Redis.Enabled {
		redisCache, err := cache.NewRedisCache(cache.Config{
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
			pkglogger.Info(fmt.Sprintf("Connected to Redis at %s:%d", cfg.Redis.Host, cfg.Redis.Port))
			handlers.SetCache(redisCache)
			defer redisCache.Close()
		}
	} else {
		pkglogger.Info("Redis caching is disabled")
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		AppName:      "BEON-IPQuality API v" + version,
		// Disable startup message in production
		DisableStartupMessage: cfg.Env == "production",
	})

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

func setupMiddleware(app *fiber.App, cfg *config.Config) {
	// Recovery middleware
	app.Use(recover.New())

	// Logger middleware
	app.Use(logger.New(logger.Config{
		Format:     "[${time}] ${status} - ${method} ${path} (${latency})\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))

	// CORS middleware
	if cfg.API.CORS.Enabled {
		app.Use(cors.New(cors.Config{
			AllowOrigins: joinStrings(cfg.API.CORS.AllowOrigins),
			AllowMethods: joinStrings(cfg.API.CORS.AllowMethods),
			AllowHeaders: joinStrings(cfg.API.CORS.AllowHeaders),
		}))
	}

	// Rate limiter middleware
	if cfg.API.RateLimit > 0 {
		app.Use(limiter.New(limiter.Config{
			Max:        cfg.API.RateLimit,
			Expiration: cfg.API.RateLimitWindow,
			KeyGenerator: func(c *fiber.Ctx) string {
				// Use API key if available, otherwise use IP
				apiKey := c.Get("X-API-Key")
				if apiKey != "" {
					return apiKey
				}
				return c.IP()
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error":   "rate_limit_exceeded",
					"message": "Too many requests. Please try again later.",
				})
			},
		}))
	}
}

func setupRoutes(app *fiber.App, cfg *config.Config) {
	// Health check endpoint (no auth required)
	if cfg.Health.Enabled {
		app.Get(cfg.Health.Path, handlers.HealthCheck(version))
	}

	// Prometheus metrics endpoint (no auth required)
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// API v1 routes
	v1 := app.Group("/api/v1")

	// Apply API key authentication if enabled
	if cfg.API.AuthEnabled {
		v1.Use(middleware.APIKeyAuth())
	}

	// IP check endpoints
	v1.Get("/check/:ip", handlers.CheckIP())

	if cfg.API.BatchEnabled {
		v1.Post("/check/batch", handlers.BatchCheckIP(cfg.API.BatchMaxSize))
	}

	// Stats endpoint
	v1.Get("/stats", handlers.GetStats())

	// Cache endpoints
	v1.Get("/cache/stats", handlers.GetCacheStats())
	v1.Delete("/cache", handlers.ClearCache())

	// Hot reload endpoint (for admin use)
	v1.Post("/reload", handlers.ReloadMMDB())

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
