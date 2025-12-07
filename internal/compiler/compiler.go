package compiler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/internal/scoring"
	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// Compiler compiles IP reputation data into MMDB format
type Compiler struct {
	config      *config.Config
	db          *pgxpool.Pool
	mmdbWriter  *mmdb.Writer
	scorer      *scoring.Scorer
	mu          sync.Mutex
	lastCompile time.Time
}

// New creates a new Compiler instance
func New(cfg *config.Config) (*Compiler, error) {
	// Connect to PostgreSQL
	poolConfig, err := pgxpool.ParseConfig(cfg.Database.Postgres.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.Database.Postgres.MaxConnections)
	poolConfig.MinConns = int32(cfg.Database.Postgres.MinConnections)

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create MMDB writer
	writerConfig := mmdb.WriterConfig{
		DatabaseType:        "BEON-IPReputation",
		Description:         "BEON IP Reputation Database",
		RecordSize:          cfg.MMDB.RecordSize,
		IPVersion:           0,
		IncludeReservedNets: false,
	}
	mmdbWriter := mmdb.NewWriter(writerConfig)

	// Create scorer
	scorer := scoring.NewDefault()

	return &Compiler{
		config:     cfg,
		db:         pool,
		mmdbWriter: mmdbWriter,
		scorer:     scorer,
	}, nil
}

// Close closes database connections
func (c *Compiler) Close() {
	if c.db != nil {
		c.db.Close()
	}
}

// Compile compiles the reputation database to MMDB
func (c *Compiler) Compile(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	logger.Info("Starting MMDB compilation...")
	startTime := time.Now()

	// Fetch reputation data from database
	reputations, err := c.fetchReputationData(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch reputation data: %w", err)
	}

	logger.Info(fmt.Sprintf("Fetched %d reputation entries from database", len(reputations)))

	if len(reputations) == 0 {
		logger.Warn("No reputation data to compile")
		return nil
	}

	// Calculate risk scores for all entries
	now := time.Now()
	for i := range reputations {
		threats := []models.Threat{{
			ThreatType: reputations[i].ThreatType,
			Source:     reputations[i].Source,
			Confidence: reputations[i].Confidence,
			LastSeen:   reputations[i].LastSeen,
			Weight:     reputations[i].Weight,
		}}

		score := c.scorer.CalculateScore(threats, nil, now)
		reputations[i].RiskScore = score
	}

	// Compile to MMDB
	outputPath := c.config.MMDB.OutputPath
	if err := c.mmdbWriter.CompileFromIPReputations(reputations, outputPath); err != nil {
		return fmt.Errorf("failed to compile MMDB: %w", err)
	}

	c.lastCompile = time.Now()
	logger.Info(fmt.Sprintf("MMDB compilation complete in %v, output: %s", time.Since(startTime), outputPath))

	// Notify judge nodes about new database (if configured)
	if c.config.Judge.Enabled {
		c.notifyJudgeNodes()
	}

	return nil
}

// fetchReputationData fetches all active reputation data from the database
func (c *Compiler) fetchReputationData(ctx context.Context) ([]models.IPReputation, error) {
	query := `
		SELECT 
			id,
			COALESCE(cidr::text, ip_start::text || '/32') as ip_range,
			source,
			threat_type,
			confidence,
			weight,
			first_seen,
			last_seen
		FROM ip_reputation
		WHERE (expires_at IS NULL OR expires_at > NOW())
		ORDER BY last_seen DESC
	`

	rows, err := c.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var reputations []models.IPReputation

	for rows.Next() {
		var rep models.IPReputation
		err := rows.Scan(
			&rep.ID,
			&rep.IPRange,
			&rep.Source,
			&rep.ThreatType,
			&rep.Confidence,
			&rep.Weight,
			&rep.FirstSeen,
			&rep.LastSeen,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to scan row: %v", err))
			continue
		}
		reputations = append(reputations, rep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return reputations, nil
}

// notifyJudgeNodes sends notification to judge nodes about new MMDB
func (c *Compiler) notifyJudgeNodes() {
	// TODO: Implement notification mechanism
	// Options: Redis pub/sub, HTTP webhook, gRPC, etc.
	logger.Debug("Judge node notification not yet implemented")
}

// GetLastCompileTime returns the last compilation time
func (c *Compiler) GetLastCompileTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastCompile
}

// Stats returns compilation statistics
type Stats struct {
	LastCompile  time.Time     `json:"last_compile"`
	TotalEntries int           `json:"total_entries"`
	CompileCount int           `json:"compile_count"`
	LastDuration time.Duration `json:"last_duration"`
}

func (c *Compiler) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{
		LastCompile: c.lastCompile,
	}
}
