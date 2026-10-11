package handlers

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lfrfrfr/beon-ipquality/internal/cache"
	internalmetrics "github.com/lfrfrfr/beon-ipquality/internal/metrics"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/pkg/iputil"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

var cacheRetryAt atomic.Int64

var ErrReputationUnavailable = errors.New("reputation database unavailable")

var (
	mmdbReader   *mmdb.Reader
	mmdbMu       sync.RWMutex
	ipCache      cache.Cache
	cacheMu      sync.RWMutex
	cacheTimeout = 50 * time.Millisecond
)

// SetMMDBReader sets the MMDB reader for IP lookups
func SetMMDBReader(reader *mmdb.Reader) {
	mmdbMu.Lock()
	defer mmdbMu.Unlock()
	mmdbReader = reader
}

// getMMDBReader returns the current MMDB reader
func getMMDBReader() *mmdb.Reader {
	mmdbMu.RLock()
	defer mmdbMu.RUnlock()
	return mmdbReader
}

// SetCache sets the cache instance
func SetCache(c cache.Cache) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	ipCache = c
	cacheRetryAt.Store(0)
}

// getCache returns the current cache instance
func getCache() cache.Cache {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return ipCache
}

// GetConfiguredCache exposes the configured cache to service lifecycle code.
func GetConfiguredCache() cache.Cache {
	return getCache()
}

// CheckIP handles single IP reputation check
func CheckIP() fiber.Handler {
	return func(c *fiber.Ctx) error {
		startTime := time.Now()

		ipParam := c.Params("ip")
		if ipParam == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "invalid_request",
				"message": "IP address is required",
			})
		}

		// Parse IP address
		addr, err := iputil.ParseIP(ipParam)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "invalid_ip",
				"message": "Invalid IP address format",
			})
		}

		// Normalize IP (IPv4-mapped IPv6 to IPv4)
		addr = iputil.NormalizeIP(addr)

		// Check if IP is valid for reputation checking
		if !iputil.IsValid(addr) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "invalid_ip",
				"message": "IP address is not suitable for reputation check (private, loopback, etc.)",
			})
		}

		result, err := performIPCheck(addr, startTime)
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":   "reputation_unavailable",
				"message": "Reputation data is temporarily unavailable",
			})
		}

		return c.JSON(result)
	}
}

// BatchCheckIP handles batch IP reputation check
func BatchCheckIP(maxSize int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		startTime := time.Now()

		var req models.BatchCheckRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "invalid_request",
				"message": "Invalid request body",
			})
		}

		if len(req.IPs) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "invalid_request",
				"message": "At least one IP address is required",
			})
		}

		if len(req.IPs) > maxSize {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "too_many_ips",
				"message": "Exceeded maximum batch size",
				"max":     maxSize,
			})
		}

		results := make([]models.IPCheckResult, 0, len(req.IPs))

		for _, ipStr := range req.IPs {
			ipStartTime := time.Now()

			addr, err := iputil.ParseIP(ipStr)
			if err != nil {
				results = append(results, models.IPCheckResult{
					IP:        ipStr,
					Score:     -1,
					RiskLevel: "error",
				})
				continue
			}

			addr = iputil.NormalizeIP(addr)

			if !iputil.IsValid(addr) {
				results = append(results, models.IPCheckResult{
					IP:        ipStr,
					Score:     -1,
					RiskLevel: "invalid",
				})
				continue
			}

			result, err := performIPCheck(addr, ipStartTime)
			if err != nil {
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
					"error":   "reputation_unavailable",
					"message": "Reputation data is temporarily unavailable",
				})
			}
			results = append(results, result)
		}

		return c.JSON(models.BatchCheckResponse{
			Results:    results,
			TotalTime:  float64(time.Since(startTime).Microseconds()) / 1000.0,
			TotalCount: len(results),
		})
	}
}

// GetStats returns API usage statistics
func GetStats() fiber.Handler {
	return func(c *fiber.Ctx) error {
		snapshot := internalmetrics.Stats()
		stats := models.APIStats{
			TotalRequests:   snapshot.TotalRequests,
			TotalIPs:        snapshot.TotalIPs,
			AvgResponseTime: snapshot.AvgResponseTime,
			ErrorRate:       snapshot.ErrorRate,
			Period:          "process_lifetime",
		}

		return c.JSON(stats)
	}
}

// HealthCheck returns health status
func HealthCheck(version string) fiber.Handler {
	startTime := time.Now()

	return func(c *fiber.Ctx) error {
		reader := getMMDBReader()
		statusCode := fiber.StatusOK
		statusText := "healthy"
		mmdbStatus := "healthy"
		var snapshot *mmdb.SnapshotStatus
		if reader != nil && reader.IsReady() {
			value := reader.Snapshot(getMMDBConfig().MaxAge, time.Now())
			snapshot = &value
			internalmetrics.SnapshotBuiltAt.Set(float64(value.BuiltAt.Unix()))
			stale := 0.0
			if value.Stale {
				stale = 1
			}
			internalmetrics.SnapshotStale.Set(stale)
		}
		if reader == nil || !reader.IsReady() {
			statusCode = fiber.StatusServiceUnavailable
			statusText = "unhealthy"
			mmdbStatus = "unavailable"
		}

		if snapshot != nil && snapshot.Stale {
			statusCode = fiber.StatusServiceUnavailable
			statusText = "unhealthy"
			mmdbStatus = "stale"
		}

		status := models.HealthStatus{
			Status:    statusText,
			Version:   version,
			Uptime:    time.Since(startTime).String(),
			Timestamp: time.Now(),
			Services: map[string]string{
				"api":  "healthy",
				"mmdb": mmdbStatus,
			},
		}

		return c.Status(statusCode).JSON(fiber.Map{"status": status.Status, "version": status.Version, "uptime": status.Uptime, "timestamp": status.Timestamp, "services": status.Services, "snapshot": snapshot})
	}
}

// performIPCheck performs the actual IP reputation check using MMDB with caching
func performIPCheck(addr netip.Addr, startTime time.Time) (models.IPCheckResult, error) {
	ipStr := addr.String()

	reader := getMMDBReader()
	if reader == nil || !reader.IsReady() {
		return models.IPCheckResult{}, ErrReputationUnavailable
	}
	snapshot := reader.Snapshot(getMMDBConfig().MaxAge, time.Now())
	if snapshot.Stale {
		return models.IPCheckResult{}, ErrReputationUnavailable
	}
	// A cached response belongs to the exact reputation/GeoIP/ASN generation.
	if cache := getCache(); cache != nil && time.Now().UnixNano() >= cacheRetryAt.Load() {
		ctx, cancel := cacheContext()
		cached, err := cache.Get(ctx, snapshot.Version+":"+ipStr)
		cancel()
		if err != nil {
			cacheRetryAt.Store(time.Now().Add(time.Second).UnixNano())
			internalmetrics.CacheHits.WithLabelValues("get", "error").Inc()
		}
		if err == nil && cached != nil && cached.DataVersion == snapshot.Version && reader.Version() == snapshot.Version && reader.IsReady() && !reader.Snapshot(getMMDBConfig().MaxAge, time.Now()).Stale {
			cached.QueryTime = float64(time.Since(startTime).Microseconds()) / 1000
			cached.Cached = true
			internalmetrics.RecordIPCheck(cached.RiskLevel, true, cached.QueryTime, "cache")
			return *cached, nil
		}
	}

	result, err := reader.LookupAllFresh(addr, getMMDBConfig().MaxAge, time.Now())
	if err != nil || result == nil {
		return models.IPCheckResult{}, ErrReputationUnavailable
	}
	result.QueryTime = float64(time.Since(startTime).Microseconds()) / 1000.0
	result.Cached = false
	internalmetrics.RecordIPCheck(result.RiskLevel, false, result.QueryTime, "mmdb")
	internalmetrics.RiskScoreDistribution.Observe(float64(result.Score))
	for _, threatType := range result.ThreatTypes {
		internalmetrics.RecordThreatDetection(threatType)
	}

	if c := getCache(); c != nil && time.Now().UnixNano() >= cacheRetryAt.Load() {
		ctx, cancel := cacheContext()
		if err := c.Set(ctx, result.DataVersion+":"+ipStr, result); err != nil {
			cacheRetryAt.Store(time.Now().Add(time.Second).UnixNano())
			internalmetrics.CacheHits.WithLabelValues("set", "error").Inc()
		}
		cancel()
	}

	return *result, nil
}

// GetCacheStats returns cache statistics
func GetCacheStats() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cache := getCache()
		if cache == nil {
			return c.JSON(fiber.Map{
				"enabled": false,
				"message": "Cache is not enabled",
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stats, err := cache.Stats(ctx)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to get cache stats",
			})
		}

		return c.JSON(fiber.Map{
			"enabled": true,
			"stats":   stats,
		})
	}
}

// ClearCache clears all cached entries
func ClearCache() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cache := getCache()
		if cache == nil {
			return c.JSON(fiber.Map{
				"success": false,
				"message": "Cache is not enabled",
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := cache.Clear(ctx); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to clear cache",
			})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Cache cleared",
		})
	}
}

// MMDBConfig holds MMDB paths for reload
type MMDBConfig struct {
	MaxAge         time.Duration
	ReputationPath string
	GeoIPCityPath  string
	GeoIPASNPath   string
}

var mmdbConfig MMDBConfig
var mmdbConfigMu sync.RWMutex

// SetMMDBConfig sets the MMDB configuration for hot reload
func SetMMDBConfig(cfg MMDBConfig) {
	mmdbConfigMu.Lock()
	defer mmdbConfigMu.Unlock()
	mmdbConfig = cfg
}

// ReloadMMDB reloads the MMDB database without restart
func ReloadMMDB() fiber.Handler {
	return func(c *fiber.Ctx) error {
		cfg := getMMDBConfig()
		// Validate reload configuration before changing live state.
		if cfg.ReputationPath == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   "MMDB config not set",
			})
		}

		reader := getMMDBReader()
		var err error
		changed := true
		if reader == nil {
			reader, err = mmdb.NewReader(
				cfg.ReputationPath,
				cfg.GeoIPCityPath,
				cfg.GeoIPASNPath,
			)
			if err == nil {
				SetMMDBReader(reader)
			}
		} else {
			changed, err = reader.ReloadIfChanged(
				cfg.ReputationPath,
				cfg.GeoIPCityPath,
				cfg.GeoIPASNPath,
			)
		}
		if err != nil {
			internalmetrics.Reloads.WithLabelValues("error").Inc()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "Failed to reload MMDB: " + err.Error(),
			})
		}

		status := "unchanged"
		if changed {
			status = "changed"
		}
		internalmetrics.Reloads.WithLabelValues(status).Inc()

		return c.JSON(fiber.Map{
			"success":      true,
			"message":      "MMDB reload checked; cache follows snapshot version",
			"changed":      changed,
			"data_version": reader.Version(),
		})
	}
}

func getMMDBConfig() MMDBConfig {
	mmdbConfigMu.RLock()
	defer mmdbConfigMu.RUnlock()
	return mmdbConfig
}

func SetCacheTimeout(timeout time.Duration) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if timeout > 0 {
		cacheTimeout = timeout
	}
}

func cacheContext() (context.Context, context.CancelFunc) {
	cacheMu.RLock()
	timeout := cacheTimeout
	cacheMu.RUnlock()
	return context.WithTimeout(context.Background(), timeout)
}
