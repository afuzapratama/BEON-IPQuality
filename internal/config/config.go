package config

import (
	"fmt"
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
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
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
	Enabled  bool   `mapstructure:"enabled"`
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	PoolSize int    `mapstructure:"pool_size"`
}

// Addr returns the Redis address
func (r *RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

// MMDBConfig holds MMDB file configuration
type MMDBConfig struct {
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
	Enabled     bool          `mapstructure:"enabled"`
	Concurrency int           `mapstructure:"concurrency"`
	HTTPTimeout time.Duration `mapstructure:"http_timeout"`
	MaxRetries  int           `mapstructure:"max_retries"`
	RetryDelay  time.Duration `mapstructure:"retry_delay"`
	UserAgent   string        `mapstructure:"user_agent"`
}

// APIConfig holds API configuration
type APIConfig struct {
	AuthEnabled     bool          `mapstructure:"auth_enabled"`
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
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Set defaults
	setDefaults()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal config
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// LoadFromEnv loads configuration with environment variable overrides
func LoadFromEnv(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Enable environment variable overrides
	viper.AutomaticEnv()

	// Set defaults
	setDefaults()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal config
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults() {
	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.read_timeout", "5s")
	viper.SetDefault("server.write_timeout", "10s")
	viper.SetDefault("server.idle_timeout", "120s")

	// Environment
	viper.SetDefault("environment", "development")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.output", "stdout")

	// PostgreSQL defaults
	viper.SetDefault("database.postgres.host", "localhost")
	viper.SetDefault("database.postgres.port", 5432)
	viper.SetDefault("database.postgres.database", "beon_ipquality")
	viper.SetDefault("database.postgres.ssl_mode", "disable")
	viper.SetDefault("database.postgres.max_connections", 100)
	viper.SetDefault("database.postgres.min_connections", 10)

	// MMDB defaults
	viper.SetDefault("mmdb.reputation_path", "./data/mmdb/reputation.mmdb")
	viper.SetDefault("mmdb.reload_interval", "1h")
	viper.SetDefault("mmdb.memory_map", true)

	// Scoring defaults
	viper.SetDefault("scoring.decay_lambda", 0.01)
	viper.SetDefault("scoring.max_score", 100)
	viper.SetDefault("scoring.risk_threshold", 50)

	// Ingestor defaults
	viper.SetDefault("ingestor.enabled", true)
	viper.SetDefault("ingestor.concurrency", 10)
	viper.SetDefault("ingestor.http_timeout", "30s")
	viper.SetDefault("ingestor.max_retries", 3)
	viper.SetDefault("ingestor.retry_delay", "5s")
	viper.SetDefault("ingestor.user_agent", "BEON-IPQuality-Ingestor/1.0")

	// API defaults
	viper.SetDefault("api.auth_enabled", true)
	viper.SetDefault("api.rate_limit", 1000)
	viper.SetDefault("api.rate_limit_window", "1m")
	viper.SetDefault("api.batch_enabled", true)
	viper.SetDefault("api.batch_max_size", 100)

	// Metrics defaults
	viper.SetDefault("metrics.enabled", true)
	viper.SetDefault("metrics.port", 9090)
	viper.SetDefault("metrics.path", "/metrics")

	// Health defaults
	viper.SetDefault("health.enabled", true)
	viper.SetDefault("health.path", "/health")
}
