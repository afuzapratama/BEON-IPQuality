package judge

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/internal/scoring"
	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// Node represents a Judge Node that handles IP reputation lookups and active scanning
type Node struct {
	config      *config.Config
	app         *fiber.App
	mmdbReader  *mmdb.Reader
	scorer      *scoring.Scorer
	scanner     *Scanner
	mu          sync.RWMutex
	startTime   time.Time
	lookupCount uint64
	scanCount   uint64
}

// New creates a new Judge Node
func New(cfg *config.Config) (*Node, error) {
	// Create MMDB reader
	reader, err := mmdb.NewReader(
		cfg.MMDB.ReputationPath,
		cfg.MMDB.GeoLite2CityPath,
		cfg.MMDB.GeoLite2ASNPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create MMDB reader: %w", err)
	}

	// Create scorer
	scorer := scoring.NewDefault()

	// Create scanner for active probing
	scanner := NewScanner(ScannerConfig{
		Timeout:    time.Duration(cfg.Judge.ScanTimeout) * time.Second,
		MaxWorkers: cfg.Judge.ScanWorkers,
	})

	// Create Fiber app with optimized settings
	app := fiber.New(fiber.Config{
		AppName:               "BEON-Judge-Node",
		ServerHeader:          "BEON",
		DisableStartupMessage: true,
		Prefork:               cfg.Judge.Prefork,
		ReadTimeout:           cfg.Server.ReadTimeout,
		WriteTimeout:          cfg.Server.WriteTimeout,
		IdleTimeout:           cfg.Server.IdleTimeout,
		BodyLimit:             1024, // 1KB limit for judge node
	})

	// Add recovery middleware
	app.Use(recover.New())

	node := &Node{
		config:     cfg,
		app:        app,
		mmdbReader: reader,
		scorer:     scorer,
		scanner:    scanner,
		startTime:  time.Now(),
	}

	// Setup routes
	node.setupRoutes()

	return node, nil
}

// setupRoutes configures the API routes for the judge node
func (n *Node) setupRoutes() {
	// Single IP lookup - optimized for minimum latency
	n.app.Get("/check/:ip", n.handleCheck)

	// Active scanning endpoints
	n.app.Get("/scan/:ip", n.handleScan)
	n.app.Get("/scan/:ip/quick", n.handleQuickScan)

	// Internal endpoints
	n.app.Get("/health", n.handleHealth)
	n.app.Get("/stats", n.handleStats)
	n.app.Post("/reload", n.handleReload)

	// Prometheus metrics endpoint
	n.app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))
}

// Start starts the judge node server
func (n *Node) Start(ctx context.Context) error {
	// Start MMDB reload goroutine
	if n.config.MMDB.ReloadInterval > 0 {
		go n.reloadLoop(ctx)
	}

	addr := fmt.Sprintf("%s:%d", n.config.Server.Host, n.config.Judge.Port)
	return n.app.Listen(addr)
}

// Close closes the judge node
func (n *Node) Close() error {
	if n.mmdbReader != nil {
		n.mmdbReader.Close()
	}
	return n.app.Shutdown()
}

// handleCheck handles IP check requests
func (n *Node) handleCheck(c *fiber.Ctx) error {
	start := time.Now()
	ipStr := c.Params("ip")

	// Parse IP
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid IP address",
			"ip":    ipStr,
		})
	}

	// Perform lookup
	n.mu.RLock()
	result, err := n.mmdbReader.LookupAll(addr)
	n.mu.RUnlock()

	if err != nil {
		logger.Error(fmt.Sprintf("Lookup error for %s: %v", ipStr, err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Lookup failed",
			"ip":    ipStr,
		})
	}

	// Ensure we have a result
	if result == nil {
		result = &models.IPCheckResult{
			IP:        ipStr,
			Score:     0,
			RiskScore: 0,
			RiskLevel: "clean",
		}
	}

	// Add query time
	result.QueryTime = float64(time.Since(start).Microseconds()) / 1000.0 // Convert to ms

	// Increment counter (atomic would be better but this is simple)
	n.lookupCount++

	return c.JSON(result)
}

// handleHealth handles health check requests
func (n *Node) handleHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "healthy",
		"uptime":  time.Since(n.startTime).String(),
		"node":    "judge",
		"version": "1.0.0",
	})
}

// handleStats handles statistics requests
func (n *Node) handleStats(c *fiber.Ctx) error {
	n.mu.RLock()
	mmdbStats := n.mmdbReader.Stats()
	n.mu.RUnlock()

	return c.JSON(fiber.Map{
		"uptime":       time.Since(n.startTime).String(),
		"lookup_count": n.lookupCount,
		"scan_count":   n.scanCount,
		"mmdb":         mmdbStats,
	})
}

// handleReload handles MMDB reload requests
func (n *Node) handleReload(c *fiber.Ctx) error {
	logger.Info("Reload request received")

	n.mu.Lock()
	err := n.mmdbReader.Reload(
		n.config.MMDB.ReputationPath,
		n.config.MMDB.GeoLite2CityPath,
		n.config.MMDB.GeoLite2ASNPath,
	)
	n.mu.Unlock()

	if err != nil {
		logger.Error(fmt.Sprintf("Reload failed: %v", err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":  "Reload failed",
			"detail": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "success",
		"message": "MMDB databases reloaded",
	})
}

// handleScan performs active proxy scan on an IP
func (n *Node) handleScan(c *fiber.Ctx) error {
	ipStr := c.Params("ip")

	// Validate IP
	if !parseIP(ipStr).IsValid() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid IP address",
			"ip":    ipStr,
		})
	}

	// Perform scan
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := n.scanner.Scan(ctx, ipStr)
	n.scanCount++

	return c.JSON(result)
}

// handleQuickScan performs quick proxy scan on an IP
func (n *Node) handleQuickScan(c *fiber.Ctx) error {
	ipStr := c.Params("ip")

	// Validate IP
	if !parseIP(ipStr).IsValid() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid IP address",
			"ip":    ipStr,
		})
	}

	// Perform quick scan
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := n.scanner.QuickScan(ctx, ipStr)
	n.scanCount++

	return c.JSON(result)
}

// parseIP helper to validate IP address
func parseIP(ip string) netip.Addr {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}
	}
	return addr
}

// reloadLoop periodically reloads MMDB databases
func (n *Node) reloadLoop(ctx context.Context) {
	ticker := time.NewTicker(n.config.MMDB.ReloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.mu.Lock()
			err := n.mmdbReader.Reload(
				n.config.MMDB.ReputationPath,
				n.config.MMDB.GeoLite2CityPath,
				n.config.MMDB.GeoLite2ASNPath,
			)
			n.mu.Unlock()

			if err != nil {
				logger.Error(fmt.Sprintf("Periodic reload failed: %v", err))
			} else {
				logger.Debug("MMDB databases reloaded successfully")
			}
		}
	}
}
