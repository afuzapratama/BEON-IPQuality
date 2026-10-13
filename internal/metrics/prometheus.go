package metrics

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	totalRequests      atomic.Int64
	totalRequestErrors atomic.Int64
	totalCheckedIPs    atomic.Int64
	totalDurationNS    atomic.Int64
)

// Snapshot is a process-lifetime API statistics snapshot.
type Snapshot struct {
	TotalRequests   int64
	TotalIPs        int64
	AvgResponseTime float64
	ErrorRate       float64
}

var (
	// HTTPRequestsTotal counts total HTTP requests
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ipquality_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	// HTTPRequestDuration tracks request duration
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ipquality_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{.0001, .0005, .001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"method", "endpoint"},
	)

	// IPChecksTotal counts IP check operations
	IPChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ipquality_ip_checks_total",
			Help: "Total number of IP checks",
		},
		[]string{"risk_level", "cached"},
	)

	// IPCheckDuration tracks IP check duration
	IPCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ipquality_ip_check_duration_milliseconds",
			Help:    "IP check duration in milliseconds",
			Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 25, 50, 100},
		},
		[]string{"source"},
	)

	// CacheHits counts cache operations
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ipquality_cache_operations_total",
			Help: "Total cache operations",
		},
		[]string{"operation", "result"},
	)

	// CacheSize tracks current cache size
	CacheSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipquality_cache_size",
			Help: "Current number of items in cache",
		},
	)

	// MMDBEntries tracks MMDB entries
	MMDBEntries = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipquality_mmdb_entries_total",
			Help: "Total entries in MMDB database",
		},
	)

	// MMDBQueryDuration tracks MMDB query duration
	MMDBQueryDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "ipquality_mmdb_query_duration_microseconds",
			Help:    "MMDB query duration in microseconds",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
	)

	// PostgresQueryDuration tracks PostgreSQL query duration
	PostgresQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ipquality_postgres_query_duration_milliseconds",
			Help:    "PostgreSQL query duration in milliseconds",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"query_type"},
	)

	// PostgresConnections tracks active connections
	PostgresConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipquality_postgres_connections_active",
			Help: "Number of active PostgreSQL connections",
		},
	)

	// RedisOperations counts Redis operations
	RedisOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ipquality_redis_operations_total",
			Help: "Total Redis operations",
		},
		[]string{"operation", "result"},
	)

	// RedisLatency tracks Redis operation latency
	RedisLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ipquality_redis_latency_microseconds",
			Help:    "Redis operation latency in microseconds",
			Buckets: []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		},
		[]string{"operation"},
	)

	// ThreatDetections counts threat detections by type
	ThreatDetections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ipquality_threat_detections_total",
			Help: "Total threat detections by type",
		},
		[]string{"threat_type"},
	)

	// RiskScoreDistribution tracks risk score distribution
	RiskScoreDistribution = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "ipquality_risk_score_distribution",
			Help:    "Distribution of risk scores",
			Buckets: []float64{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		},
	)

	// ActiveScans tracks active scans in judge node
	ActiveScans = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "ipquality_active_scans",
			Help: "Number of active proxy scans",
		},
	)

	// ScanResults counts scan results
	ScanResults = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ipquality_scan_results_total",
			Help: "Total scan results by type",
		},
		[]string{"proxy_type", "detected"},
	)

	// ScanDuration tracks scan duration
	ScanDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ipquality_scan_duration_milliseconds",
			Help:    "Scan duration in milliseconds",
			Buckets: []float64{100, 250, 500, 1000, 2500, 5000, 10000, 30000},
		},
		[]string{"scan_type"},
	)

	// GeoIPLookups counts GeoIP lookups
	GeoIPLookups = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ipquality_geoip_lookups_total",
			Help: "Total GeoIP lookups by type",
		},
		[]string{"lookup_type", "result"},
	)

	// APIRateLimitHits counts rate limit hits
	APIRateLimitHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "ipquality_rate_limit_hits_total",
			Help: "Total rate limit hits",
		},
	)

	// ClickHouseBatchSize tracks ClickHouse batch sizes
	ClickHouseBatchSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "ipquality_clickhouse_batch_size",
			Help:    "ClickHouse batch insert sizes",
			Buckets: []float64{1, 10, 50, 100, 500, 1000, 5000},
		},
	)

	// SystemInfo provides system information
	SystemInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ipquality_system_info",
			Help: "System information",
		},
		[]string{"version", "go_version"},
	)
)

// HTTPMiddleware records route-template labels to avoid putting raw IP
// parameters into Prometheus and creating an unbounded number of time series.
func HTTPMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)
		status := c.Response().StatusCode()
		endpoint := c.Route().Path
		if endpoint == "" {
			endpoint = "unmatched"
		}
		// Fiber method strings borrow fasthttp buffers; Prometheus retains labels.
		method := strings.Clone(c.Method())
		endpoint = strings.Clone(endpoint)
		HTTPRequestsTotal.WithLabelValues(method, endpoint, strconv.Itoa(status)).Inc()
		HTTPRequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
		totalRequests.Add(1)
		totalDurationNS.Add(duration.Nanoseconds())
		if status >= 400 || err != nil {
			totalRequestErrors.Add(1)
		}
		return err
	}
}

// Stats returns process-lifetime request statistics without scraping Prometheus.
func Stats() Snapshot {
	requests := totalRequests.Load()
	errors := totalRequestErrors.Load()
	snapshot := Snapshot{TotalRequests: requests, TotalIPs: totalCheckedIPs.Load()}
	if requests > 0 {
		snapshot.AvgResponseTime = float64(totalDurationNS.Load()) / float64(requests) / float64(time.Millisecond)
		snapshot.ErrorRate = float64(errors) / float64(requests)
	}
	return snapshot
}

// RecordIPCheck records an IP check metric
func RecordIPCheck(riskLevel string, cached bool, durationMs float64, source string) {
	totalCheckedIPs.Add(1)
	cachedStr := "false"
	if cached {
		cachedStr = "true"
	}
	IPChecksTotal.WithLabelValues(riskLevel, cachedStr).Inc()
	IPCheckDuration.WithLabelValues(source).Observe(durationMs)
}

// RecordCacheOperation records a cache operation
func RecordCacheOperation(operation string, hit bool) {
	result := "miss"
	if hit {
		result = "hit"
	}
	CacheHits.WithLabelValues(operation, result).Inc()
}

// RecordThreatDetection records a threat detection
func RecordThreatDetection(threatType string) {
	ThreatDetections.WithLabelValues(threatType).Inc()
}

// RecordScanResult records a scan result
func RecordScanResult(proxyType string, detected bool, durationMs float64) {
	detectedStr := "false"
	if detected {
		detectedStr = "true"
	}
	ScanResults.WithLabelValues(proxyType, detectedStr).Inc()
	ScanDuration.WithLabelValues(proxyType).Observe(durationMs)
}

// RecordGeoIPLookup records a GeoIP lookup
func RecordGeoIPLookup(lookupType string, success bool) {
	result := "failure"
	if success {
		result = "success"
	}
	GeoIPLookups.WithLabelValues(lookupType, result).Inc()
}
