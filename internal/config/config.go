package config

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Env        string           `mapstructure:"environment"`
	Logging    LoggingConfig    `mapstructure:"logging"`
	Database   DatabaseConfig   `mapstructure:"database"`
	ClickHouse ClickHouseConfig `mapstructure:"clickhouse"`
	Redis      RedisConfig      `mapstructure:"redis"`
	MMDB       MMDBConfig       `mapstructure:"mmdb"`
	Scoring    ScoringConfig    `mapstructure:"scoring"`
	Ingestor   IngestorConfig   `mapstructure:"ingestor"`
	API        APIConfig        `mapstructure:"api"`
	Judge      JudgeConfig      `mapstructure:"judge"`
	Metrics    MetricsConfig    `mapstructure:"metrics"`
	Health     HealthConfig     `mapstructure:"health"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	TrustedProxies []string      `mapstructure:"trusted_proxies"`
	Host           string        `mapstructure:"host"`
	Port           int           `mapstructure:"port"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout"`
	IdleTimeout    time.Duration `mapstructure:"idle_timeout"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level    string `mapstructure:"level"`
	Format   string `mapstructure:"format"`
	Output   string `mapstructure:"output"`
	FilePath string `mapstructure:"file_path"`
}

// DatabaseConfig holds database configurations
type DatabaseConfig struct {
	Postgres PostgresConfig `mapstructure:"postgres"`
}

// PostgresConfig holds PostgreSQL configuration
type PostgresConfig struct {
	URL             string        `mapstructure:"url"`
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	Database        string        `mapstructure:"database"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	SSLMode         string        `mapstructure:"ssl_mode"`
	MaxConnections  int           `mapstructure:"max_connections"`
	MinConnections  int           `mapstructure:"min_connections"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time"`
}

// DSN returns the PostgreSQL connection string
func (p *PostgresConfig) DSN() string {
	if p.URL != "" {
		return p.URL
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.Host, p.Port, p.Username, p.Password, p.Database, p.SSLMode,
	)
}

// ClickHouseConfig holds ClickHouse configuration
type ClickHouseConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	OperationTimeout time.Duration `mapstructure:"operation_timeout"`
	URL              string        `mapstructure:"url"`
	Enabled          bool          `mapstructure:"enabled"`
	Host             string        `mapstructure:"host"`
	Port             int           `mapstructure:"port"`
	Password         string        `mapstructure:"password"`
	DB               int           `mapstructure:"db"`
	PoolSize         int           `mapstructure:"pool_size"`
}

// Addr returns the Redis address
func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// MMDBConfig holds MMDB file configuration
type MMDBConfig struct {
	FeedsPath        string        `mapstructure:"feeds_path"`
	MaxAge           time.Duration `mapstructure:"max_age"`
	ReputationPath   string        `mapstructure:"reputation_path"`
	GeoLite2CityPath string        `mapstructure:"geolite2_city_path"`
	GeoLite2ASNPath  string        `mapstructure:"geolite2_asn_path"`
	OutputPath       string        `mapstructure:"output_path"`
	ReloadInterval   time.Duration `mapstructure:"reload_interval"`
	CompileInterval  time.Duration `mapstructure:"compile_interval"`
	RecordSize       int           `mapstructure:"record_size"`
	MemoryMap        bool          `mapstructure:"memory_map"`
}

// ScoringConfig holds risk scoring configuration
type ScoringConfig struct {
	DecayLambda   float64        `mapstructure:"decay_lambda"`
	MaxScore      int            `mapstructure:"max_score"`
	RiskThreshold int            `mapstructure:"risk_threshold"`
	Weights       map[string]int `mapstructure:"weights"`
	ASNBonuses    map[string]int `mapstructure:"asn_bonuses"`
}

// IngestorConfig holds ingestor service configuration
type IngestorConfig struct {
	ReputationRetention time.Duration `mapstructure:"reputation_retention"`
	HistoryRetention    time.Duration `mapstructure:"history_retention"`
	SkipInitial         bool          `mapstructure:"skip_initial"`
	Enabled             bool          `mapstructure:"enabled"`
	Concurrency         int           `mapstructure:"concurrency"`
	HTTPTimeout         time.Duration `mapstructure:"http_timeout"`
	MaxRetries          int           `mapstructure:"max_retries"`
	RetryDelay          time.Duration `mapstructure:"retry_delay"`
	UserAgent           string        `mapstructure:"user_agent"`
}

// APIConfig holds API configuration
type APIConfig struct {
	AuthEnabled     bool          `mapstructure:"auth_enabled"`
	APIKey          string        `mapstructure:"api_key"`
	AdminKey        string        `mapstructure:"admin_key"`
	RateLimit       int           `mapstructure:"rate_limit"`
	RateLimitWindow time.Duration `mapstructure:"rate_limit_window"`
	BatchEnabled    bool          `mapstructure:"batch_enabled"`
	BatchMaxSize    int           `mapstructure:"batch_max_size"`
	CORS            CORSConfig    `mapstructure:"cors"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	AllowOrigins []string `mapstructure:"allow_origins"`
	AllowMethods []string `mapstructure:"allow_methods"`
	AllowHeaders []string `mapstructure:"allow_headers"`
}

// JudgeConfig holds judge node configuration
type JudgeConfig struct {
	Enabled     bool          `mapstructure:"enabled"`
	Port        int           `mapstructure:"port"`
	Prefork     bool          `mapstructure:"prefork"`
	Concurrency int           `mapstructure:"concurrency"`
	Timeout     time.Duration `mapstructure:"timeout"`
	ScanPorts   []int         `mapstructure:"scan_ports"`
	ScanTimeout int           `mapstructure:"scan_timeout"`
	ScanWorkers int           `mapstructure:"scan_workers"`
	RateLimit   int           `mapstructure:"rate_limit"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Host    string `mapstructure:"host"`
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// HealthConfig holds health check configuration
type HealthConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Path    string `mapstructure:"path"`
}

// Load loads configuration from file
func Load(configPath string) (*Config, error) {
	return load(configPath, false)
}

// LoadFromEnv loads configuration with environment variable overrides
func LoadFromEnv(configPath string) (*Config, error) {
	return load(configPath, true)
}

func load(configPath string, withEnv bool) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")
	setDefaults(v)

	if withEnv {
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
		bindEnvironment(v)
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Validate rejects configurations that would make a production deployment
// silently insecure or unable to serve correct reputation results.
func (c *Config) Validate() error {
	for _, proxy := range c.Server.TrustedProxies {
		if _, err := netip.ParseAddr(proxy); err != nil {
			prefix, prefixErr := netip.ParsePrefix(proxy)
			if prefixErr != nil || prefix.Bits() == 0 {
				return fmt.Errorf("invalid configuration: trusted proxy must be an IP or restricted CIDR")
			}
		}
	}
	if c.Scoring.RiskThreshold < 0 || c.Scoring.RiskThreshold > 100 {
		return fmt.Errorf("invalid configuration: risk threshold must be between 0 and 100")
	}
	if c.Scoring.DecayLambda < 0 || c.Scoring.MaxScore < 0 || c.Scoring.MaxScore > 100 {
		return fmt.Errorf("invalid configuration: scoring decay must be nonnegative and max score between 0 and 100")
	}
	for _, weight := range c.Scoring.Weights {
		if weight < 0 || weight > 100 {
			return fmt.Errorf("invalid configuration: source weights must be between 0 and 100")
		}
	}
	if c.MMDB.MaxAge < 0 || c.MMDB.CompileInterval < 0 || c.MMDB.ReloadInterval < 0 {
		return fmt.Errorf("invalid configuration: MMDB durations must be nonnegative")
	}
	if c.Redis.Enabled && c.Redis.OperationTimeout <= 0 {
		return fmt.Errorf("invalid configuration: redis.operation_timeout must be positive")
	}
	if c.API.AdminKey != "" && (len(c.API.AdminKey) < 32 || c.API.AdminKey == c.API.APIKey) {
		return fmt.Errorf("invalid configuration: admin key must be at least 32 characters and differ from lookup key")
	}
	if c.API.RateLimit < 0 || (c.API.RateLimit > 0 && c.API.RateLimitWindow <= 0) {
		return fmt.Errorf("invalid configuration: rate limit must be nonnegative with a positive window")
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid configuration: server.port must be between 1 and 65535")
	}
	if c.Ingestor.ReputationRetention < 0 || c.Ingestor.HistoryRetention < 0 {
		return fmt.Errorf("invalid configuration: retention cannot be negative")
	}
	if c.Ingestor.Concurrency < 1 {
		return fmt.Errorf("invalid configuration: ingestor.concurrency must be at least 1")
	}
	if c.API.BatchMaxSize < 1 {
		return fmt.Errorf("invalid configuration: api.batch_max_size must be at least 1")
	}
	if c.API.AuthEnabled && len(c.API.APIKey) < 32 {
		return fmt.Errorf("invalid configuration: api.api_key must contain at least 32 characters when authentication is enabled")
	}
	if strings.EqualFold(c.Env, "production") && !c.API.AuthEnabled {
		return fmt.Errorf("invalid configuration: api authentication cannot be disabled in production")
	}
	if strings.EqualFold(c.Env, "production") && c.API.CORS.Enabled {
		for _, origin := range c.API.CORS.AllowOrigins {
			if origin == "*" {
				return fmt.Errorf("invalid configuration: wildcard CORS cannot be enabled in production")
			}
		}
	}
	return nil
}

func bindEnvironment(v *viper.Viper) {
	bindings := map[string][]string{
		"environment":                       {"APP_ENV", "ENVIRONMENT"},
		"server.host":                       {"SERVER_HOST", "API_HOST"},
		"server.trusted_proxies":            {"TRUSTED_PROXIES"},
		"ingestor.concurrency":              {"INGESTOR_CONCURRENCY"},
		"ingestor.reputation_retention":     {"REPUTATION_RETENTION"},
		"ingestor.history_retention":        {"FEED_HISTORY_RETENTION"},
		"ingestor.skip_initial":             {"INGESTOR_SKIP_INITIAL"},
		"api.admin_key":                     {"ADMIN_API_KEY"},
		"server.port":                       {"SERVER_PORT", "API_PORT", "PORT"},
		"database.postgres.max_connections": {"DB_MAX_CONNECTIONS"},
		"database.postgres.min_connections": {"DB_MIN_CONNECTIONS"},
		"database.postgres.host":            {"DB_HOST"},
		"database.postgres.url":             {"DATABASE_URL"},
		"database.postgres.port":            {"DB_PORT"},
		"database.postgres.database":        {"DB_NAME", "POSTGRES_DB"},
		"database.postgres.username":        {"DB_USER", "POSTGRES_USER"},
		"database.postgres.password":        {"DB_PASSWORD", "POSTGRES_PASSWORD"},
		"database.postgres.ssl_mode":        {"DB_SSL_MODE"},
		"redis.operation_timeout":           {"REDIS_OPERATION_TIMEOUT"},
		"mmdb.feeds_path":                   {"FEEDS_PATH"},
		"mmdb.max_age":                      {"MMDB_MAX_AGE"},
		"mmdb.compile_interval":             {"MMDB_COMPILE_INTERVAL"},
		"mmdb.reload_interval":              {"MMDB_RELOAD_INTERVAL"},
		"redis.enabled":                     {"REDIS_ENABLED"},
		"redis.url":                         {"REDIS_URL"},
		"redis.host":                        {"REDIS_HOST"},
		"redis.port":                        {"REDIS_PORT"},
		"redis.password":                    {"REDIS_PASSWORD"},
		"mmdb.reputation_path":              {"MMDB_PATH"},
		"mmdb.geolite2_city_path":           {"GEOIP_CITY_PATH"},
		"mmdb.geolite2_asn_path":            {"GEOIP_ASN_PATH"},
		"mmdb.output_path":                  {"MMDB_OUTPUT"},
		"api.auth_enabled":                  {"API_AUTH_ENABLED"},
		"api.api_key":                       {"API_KEY", "API_MASTER_KEY"},
		"api.rate_limit":                    {"RATE_LIMIT"},
		"api.rate_limit_window":             {"RATE_LIMIT_WINDOW"},
		"api.cors.enabled":                  {"API_CORS_ENABLED"},
		"api.cors.allow_origins":            {"API_CORS_ALLOW_ORIGINS"},
		"judge.port":                        {"JUDGE_PORT"},
		"judge.scan_timeout":                {"SCAN_TIMEOUT_SECONDS"},
		"judge.scan_workers":                {"SCAN_WORKERS"},
		"judge.rate_limit":                  {"JUDGE_RATE_LIMIT"},
		"metrics.host":                      {"METRICS_HOST"},
		"metrics.port":                      {"METRICS_PORT"},
		"metrics.enabled":                   {"METRICS_ENABLED"},
		"metrics.path":                      {"METRICS_PATH"},
		"logging.level":                     {"LOG_LEVEL"},
		"logging.format":                    {"LOG_FORMAT"},
		"clickhouse.enabled":                {"CLICKHOUSE_ENABLED"},
		"clickhouse.host":                   {"CLICKHOUSE_HOST"},
		"clickhouse.port":                   {"CLICKHOUSE_PORT"},
		"clickhouse.database":               {"CLICKHOUSE_DATABASE"},
		"clickhouse.username":               {"CLICKHOUSE_USERNAME"},
		"clickhouse.password":               {"CLICKHOUSE_PASSWORD"},
	}
	for key, envNames := range bindings {
		args := append([]string{key}, envNames...)
		_ = v.BindEnv(args...)
	}
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.trusted_proxies", []string{})
	v.SetDefault("api.admin_key", "")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "5s")
	v.SetDefault("server.write_timeout", "10s")
	v.SetDefault("server.idle_timeout", "120s")

	// Environment
	v.SetDefault("environment", "development")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// PostgreSQL defaults
	v.SetDefault("database.postgres.host", "localhost")
	v.SetDefault("database.postgres.port", 5432)
	v.SetDefault("database.postgres.database", "beon_ipquality")
	v.SetDefault("database.postgres.ssl_mode", "disable")
	v.SetDefault("database.postgres.max_connections", 100)
	v.SetDefault("database.postgres.min_connections", 10)

	// MMDB defaults
	v.SetDefault("mmdb.reputation_path", "./data/mmdb/reputation.mmdb")
	v.SetDefault("mmdb.reload_interval", "5m")
	v.SetDefault("mmdb.compile_interval", "15m")
	v.SetDefault("mmdb.max_age", "1h")
	v.SetDefault("mmdb.feeds_path", "./configs/feeds.yaml")
	v.SetDefault("redis.operation_timeout", "50ms")
	v.SetDefault("mmdb.memory_map", true)

	// Scoring defaults
	v.SetDefault("scoring.decay_lambda", 0.01)
	v.SetDefault("scoring.max_score", 100)
	v.SetDefault("scoring.risk_threshold", 50)

	// Ingestor defaults
	v.SetDefault("ingestor.enabled", true)
	v.SetDefault("ingestor.reputation_retention", "720h")
	v.SetDefault("ingestor.history_retention", "720h")
	v.SetDefault("ingestor.skip_initial", false)
	v.SetDefault("ingestor.concurrency", 10)
	v.SetDefault("ingestor.http_timeout", "30s")
	v.SetDefault("ingestor.max_retries", 3)
	v.SetDefault("ingestor.retry_delay", "5s")
	v.SetDefault("ingestor.user_agent", "BEON-IPQuality-Ingestor/1.0")

	// API defaults
	v.SetDefault("api.auth_enabled", true)
	v.SetDefault("api.rate_limit", 1000)
	v.SetDefault("api.rate_limit_window", "1m")
	v.SetDefault("api.batch_enabled", true)
	v.SetDefault("api.batch_max_size", 100)

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.host", "127.0.0.1")
	v.SetDefault("metrics.port", 9090)
	v.SetDefault("metrics.path", "/metrics")

	// Health defaults
	v.SetDefault("health.enabled", true)
	v.SetDefault("health.path", "/health")
}
