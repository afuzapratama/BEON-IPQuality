package handlers

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lfrfrfr/beon-ipquality/internal/cache"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/pkg/iputil"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

var (
	mmdbReader *mmdb.Reader
	mmdbMu     sync.RWMutex
	ipCache    cache.Cache
	cacheMu    sync.RWMutex
	cacheCtx   = context.Background()
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
}

// getCache returns the current cache instance
func getCache() cache.Cache {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return ipCache
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

		// TODO: Implement actual reputation lookup from MMDB/database
		// For now, return a placeholder response
		result := performIPCheck(addr, startTime)

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

			result := performIPCheck(addr, ipStartTime)
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
		// TODO: Implement actual stats from ClickHouse or Redis
		stats := models.APIStats{
			TotalRequests:   0,
			TotalIPs:        0,
			AvgResponseTime: 0,
			ErrorRate:       0,
			Period:          "24h",
		}

		return c.JSON(stats)
	}
}

// HealthCheck returns health status
func HealthCheck(version string) fiber.Handler {
	startTime := time.Now()

	return func(c *fiber.Ctx) error {
		status := models.HealthStatus{
			Status:    "healthy",
			Version:   version,
			Uptime:    time.Since(startTime).String(),
			Timestamp: time.Now(),
			Services: map[string]string{
				"api":      "healthy",
				"database": "unknown", // TODO: Check database connection
				"mmdb":     "unknown", // TODO: Check MMDB file
			},
		}

		return c.JSON(status)
	}
}

// performIPCheck performs the actual IP reputation check using MMDB with caching
func performIPCheck(addr netip.Addr, startTime time.Time) models.IPCheckResult {
	ipStr := addr.String()

	// Try cache first
	if c := getCache(); c != nil {
		if cached, err := c.Get(cacheCtx, ipStr); err == nil && cached != nil {
			cached.QueryTime = float64(time.Since(startTime).Microseconds()) / 1000.0
			cached.Cached = true
			return *cached
		}
	}

	reader := getMMDBReader()

	// If MMDB is loaded, use it for lookup
	if reader != nil {
		result, err := reader.LookupAll(addr)
		if err == nil && result != nil {
			result.QueryTime = float64(time.Since(startTime).Microseconds()) / 1000.0
			result.Cached = false

			// Store in cache
			if c := getCache(); c != nil {
				_ = c.Set(cacheCtx, ipStr, result)
			}

			return *result
		}
	}

	// Fallback: return clean result if MMDB not available or IP not found
	result := models.IPCheckResult{
		IP:           addr.String(),
		Score:        0,
		RiskScore:    0,
		RiskLevel:    "clean",
		IsProxy:      false,
		IsVPN:        false,
		IsTor:        false,
		IsDatacenter: false,
		IsBotnet:     false,
		IsSpam:       false,
		Threats:      []models.Threat{},
		Geo:          nil,
		ASN:          nil,
		QueryTime:    float64(time.Since(startTime).Microseconds()) / 1000.0,
		Cached:       false,
	}

	// Cache clean results too (shorter TTL would be better for these)
	if c := getCache(); c != nil {
		_ = c.Set(cacheCtx, ipStr, &result)
	}

	return result
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

		stats, err := cache.Stats(cacheCtx)
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

		if err := cache.Clear(cacheCtx); err != nil {
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
	ReputationPath string
	GeoIPCityPath  string
	GeoIPASNPath   string
}

var mmdbConfig MMDBConfig

// SetMMDBConfig sets the MMDB configuration for hot reload
func SetMMDBConfig(cfg MMDBConfig) {
	mmdbConfig = cfg
}

// ReloadMMDB reloads the MMDB database without restart
func ReloadMMDB() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Clear cache first
		if cache := getCache(); cache != nil {
			_ = cache.Clear(cacheCtx)
		}

		// Try to reload MMDB
		if mmdbConfig.ReputationPath == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   "MMDB config not set",
			})
		}

		newReader, err := mmdb.NewReader(
			mmdbConfig.ReputationPath,
			mmdbConfig.GeoIPCityPath,
			mmdbConfig.GeoIPASNPath,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "Failed to reload MMDB: " + err.Error(),
			})
		}

		// Swap readers
		mmdbMu.Lock()
		oldReader := mmdbReader
		mmdbReader = newReader
		mmdbMu.Unlock()

		// Close old reader
		if oldReader != nil {
			oldReader.Close()
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "MMDB reloaded successfully, cache cleared",
		})
	}
}
