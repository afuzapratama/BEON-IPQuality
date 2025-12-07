package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// Client handles ClickHouse operations
type Client struct {
	conn     driver.Conn
	database string
	batch    []APIRequestLog
	batchMu  chan struct{}
}

// Config holds ClickHouse configuration
type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
}

// APIRequestLog represents a single API request log entry
type APIRequestLog struct {
	Timestamp    time.Time
	IPChecked    string
	ClientIP     string
	APIKey       string
	Endpoint     string
	Method       string
	RiskScore    uint8
	RiskLevel    string
	IsProxy      bool
	IsVPN        bool
	IsTor        bool
	IsDatacenter bool
	IsBotnet     bool
	CountryCode  string
	Country      string
	City         string
	ASN          uint32
	ASNOrg       string
	QueryTimeMs  float32
	Cached       bool
	UserAgent    string
	ResponseCode uint16
}

// ScanResultLog represents a scan result log entry
type ScanResultLog struct {
	Timestamp     time.Time
	IP            string
	IsProxy       bool
	IsSOCKS4      bool
	IsSOCKS5      bool
	IsHTTPProxy   bool
	IsHTTPConnect bool
	OpenPorts     []uint16
	ProxyPorts    []uint16
	ScanTimeMs    float32
}

// NewClient creates a new ClickHouse client
func NewClient(cfg Config) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     5 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	logger.Info(fmt.Sprintf("Connected to ClickHouse at %s:%d", cfg.Host, cfg.Port))

	return &Client{
		conn:     conn,
		database: cfg.Database,
		batch:    make([]APIRequestLog, 0, 1000),
		batchMu:  make(chan struct{}, 1),
	}, nil
}

// LogRequest logs an API request
func (c *Client) LogRequest(ctx context.Context, log APIRequestLog) error {
	query := `
		INSERT INTO api_requests (
			timestamp, ip_checked, client_ip, api_key, endpoint, method,
			risk_score, risk_level, is_proxy, is_vpn, is_tor, is_datacenter, is_botnet,
			country_code, country, city, asn, asn_org,
			query_time_ms, cached, user_agent, response_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return c.conn.Exec(ctx, query,
		log.Timestamp, log.IPChecked, log.ClientIP, log.APIKey, log.Endpoint, log.Method,
		log.RiskScore, log.RiskLevel, log.IsProxy, log.IsVPN, log.IsTor, log.IsDatacenter, log.IsBotnet,
		log.CountryCode, log.Country, log.City, log.ASN, log.ASNOrg,
		log.QueryTimeMs, log.Cached, log.UserAgent, log.ResponseCode,
	)
}

// LogRequestAsync logs an API request asynchronously (batched)
func (c *Client) LogRequestAsync(log APIRequestLog) {
	select {
	case c.batchMu <- struct{}{}:
		c.batch = append(c.batch, log)
		if len(c.batch) >= 100 {
			go c.flushBatch()
		}
		<-c.batchMu
	default:
		// Channel busy, skip this log
	}
}

// flushBatch writes batched logs to ClickHouse
func (c *Client) flushBatch() {
	c.batchMu <- struct{}{}
	defer func() { <-c.batchMu }()

	if len(c.batch) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO api_requests (
			timestamp, ip_checked, client_ip, api_key, endpoint, method,
			risk_score, risk_level, is_proxy, is_vpn, is_tor, is_datacenter, is_botnet,
			country_code, country, city, asn, asn_org,
			query_time_ms, cached, user_agent, response_code
		)
	`)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to prepare batch: %v", err))
		return
	}

	for _, log := range c.batch {
		err := batch.Append(
			log.Timestamp, log.IPChecked, log.ClientIP, log.APIKey, log.Endpoint, log.Method,
			log.RiskScore, log.RiskLevel, log.IsProxy, log.IsVPN, log.IsTor, log.IsDatacenter, log.IsBotnet,
			log.CountryCode, log.Country, log.City, log.ASN, log.ASNOrg,
			log.QueryTimeMs, log.Cached, log.UserAgent, log.ResponseCode,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to append to batch: %v", err))
		}
	}

	if err := batch.Send(); err != nil {
		logger.Error(fmt.Sprintf("Failed to send batch: %v", err))
	}

	c.batch = c.batch[:0]
}

// LogScanResult logs a scan result
func (c *Client) LogScanResult(ctx context.Context, log ScanResultLog) error {
	query := `
		INSERT INTO scan_results (
			timestamp, ip, is_proxy, is_socks4, is_socks5, 
			is_http_proxy, is_http_connect, open_ports, proxy_ports, scan_time_ms
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return c.conn.Exec(ctx, query,
		log.Timestamp, log.IP, log.IsProxy, log.IsSOCKS4, log.IsSOCKS5,
		log.IsHTTPProxy, log.IsHTTPConnect, log.OpenPorts, log.ProxyPorts, log.ScanTimeMs,
	)
}

// GetHourlyStats retrieves hourly statistics
func (c *Client) GetHourlyStats(ctx context.Context, hours int) ([]HourlyStats, error) {
	query := `
		SELECT 
			toStartOfHour(timestamp) AS hour,
			count() AS total_requests,
			uniq(ip_checked) AS unique_ips,
			avg(query_time_ms) AS avg_query_time,
			countIf(cached) * 100.0 / count() AS cache_hit_rate
		FROM api_requests
		WHERE timestamp >= now() - INTERVAL ? HOUR
		GROUP BY hour
		ORDER BY hour DESC
	`

	rows, err := c.conn.Query(ctx, query, hours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []HourlyStats
	for rows.Next() {
		var s HourlyStats
		if err := rows.Scan(&s.Hour, &s.TotalRequests, &s.UniqueIPs, &s.AvgQueryTime, &s.CacheHitRate); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}

	return stats, nil
}

// HourlyStats represents hourly statistics
type HourlyStats struct {
	Hour          time.Time `json:"hour"`
	TotalRequests uint64    `json:"total_requests"`
	UniqueIPs     uint64    `json:"unique_ips"`
	AvgQueryTime  float32   `json:"avg_query_time_ms"`
	CacheHitRate  float32   `json:"cache_hit_rate"`
}

// GetTopThreats retrieves top threats
func (c *Client) GetTopThreats(ctx context.Context, limit int) ([]TopThreat, error) {
	query := `
		SELECT 
			ip_checked,
			max(risk_score) AS max_risk,
			any(risk_level) AS risk_level,
			any(country_code) AS country,
			any(asn) AS asn,
			any(asn_org) AS asn_org,
			count() AS hit_count
		FROM api_requests
		WHERE timestamp >= now() - INTERVAL 24 HOUR
		  AND risk_score > 50
		GROUP BY ip_checked
		ORDER BY hit_count DESC
		LIMIT ?
	`

	rows, err := c.conn.Query(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threats []TopThreat
	for rows.Next() {
		var t TopThreat
		if err := rows.Scan(&t.IP, &t.RiskScore, &t.RiskLevel, &t.Country, &t.ASN, &t.ASNOrg, &t.HitCount); err != nil {
			return nil, err
		}
		threats = append(threats, t)
	}

	return threats, nil
}

// TopThreat represents a top threat entry
type TopThreat struct {
	IP        string `json:"ip"`
	RiskScore uint8  `json:"risk_score"`
	RiskLevel string `json:"risk_level"`
	Country   string `json:"country"`
	ASN       uint32 `json:"asn"`
	ASNOrg    string `json:"asn_org"`
	HitCount  uint64 `json:"hit_count"`
}

// GetDashboardData retrieves data for dashboard
func (c *Client) GetDashboardData(ctx context.Context) (*DashboardData, error) {
	data := &DashboardData{}

	// Total requests today
	row := c.conn.QueryRow(ctx, `
		SELECT count(), uniq(ip_checked), avg(query_time_ms)
		FROM api_requests
		WHERE timestamp >= today()
	`)
	if err := row.Scan(&data.TodayRequests, &data.TodayUniqueIPs, &data.AvgResponseTime); err != nil {
		return nil, err
	}

	// Threat distribution
	rows, err := c.conn.Query(ctx, `
		SELECT risk_level, count() 
		FROM api_requests 
		WHERE timestamp >= today() 
		GROUP BY risk_level
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	data.ThreatDistribution = make(map[string]uint64)
	for rows.Next() {
		var level string
		var count uint64
		if err := rows.Scan(&level, &count); err != nil {
			continue
		}
		data.ThreatDistribution[level] = count
	}

	return data, nil
}

// DashboardData represents dashboard data
type DashboardData struct {
	TodayRequests      uint64            `json:"today_requests"`
	TodayUniqueIPs     uint64            `json:"today_unique_ips"`
	AvgResponseTime    float32           `json:"avg_response_time_ms"`
	ThreatDistribution map[string]uint64 `json:"threat_distribution"`
}

// FromIPCheckResult converts IPCheckResult to APIRequestLog
func FromIPCheckResult(result *models.IPCheckResult, clientIP, apiKey, endpoint, method, userAgent string, responseCode uint16) APIRequestLog {
	log := APIRequestLog{
		Timestamp:    time.Now(),
		IPChecked:    result.IP,
		ClientIP:     clientIP,
		APIKey:       apiKey,
		Endpoint:     endpoint,
		Method:       method,
		RiskScore:    uint8(result.Score),
		RiskLevel:    result.RiskLevel,
		IsProxy:      result.IsProxy,
		IsVPN:        result.IsVPN,
		IsTor:        result.IsTor,
		IsDatacenter: result.IsDatacenter,
		IsBotnet:     result.IsBotnet,
		QueryTimeMs:  float32(result.QueryTime),
		Cached:       result.Cached,
		UserAgent:    userAgent,
		ResponseCode: responseCode,
	}

	if result.Geo != nil {
		log.CountryCode = result.Geo.CountryCode
		log.Country = result.Geo.Country
		log.City = result.Geo.City
	}

	if result.ASN != nil {
		log.ASN = uint32(result.ASN.ASN)
		log.ASNOrg = result.ASN.Org
	}

	return log
}

// Close closes the ClickHouse connection
func (c *Client) Close() error {
	c.flushBatch()
	return c.conn.Close()
}
