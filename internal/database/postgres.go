package database

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// PostgresDB handles PostgreSQL database operations
type PostgresDB struct {
	pool *pgxpool.Pool
}

// NewPostgresDB creates a new PostgreSQL connection pool
func NewPostgresDB(dsn string, maxConns, minConns int) (*PostgresDB, error) {
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	poolConfig.MaxConns = int32(maxConns)
	poolConfig.MinConns = int32(minConns)
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Connected to PostgreSQL database")
	return &PostgresDB{pool: pool}, nil
}

// Close closes the database connection pool
func (db *PostgresDB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Pool returns the underlying connection pool
func (db *PostgresDB) Pool() *pgxpool.Pool {
	return db.pool
}

// IPReputationEntry represents a database entry
type IPReputationEntry struct {
	ID         int64
	IPStart    string
	IPEnd      string
	CIDR       *string
	Source     string
	SourceName *string
	ThreatType string
	Confidence float64
	Weight     int
	FirstSeen  time.Time
	LastSeen   time.Time
	ExpiresAt  *time.Time
	Metadata   map[string]interface{}
}

// InsertReputation inserts or updates an IP reputation entry
func (db *PostgresDB) InsertReputation(ctx context.Context, entry *IPReputationEntry) error {
	query := `
		INSERT INTO ip_reputation (ip_start, ip_end, cidr, source, source_name, threat_type, confidence, weight, first_seen, last_seen, expires_at)
		VALUES ($1::inet, $2::inet, $3::cidr, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (ip_start, ip_end, source) 
		DO UPDATE SET
			confidence = GREATEST(ip_reputation.confidence, EXCLUDED.confidence),
			weight = GREATEST(ip_reputation.weight, EXCLUDED.weight),
			last_seen = EXCLUDED.last_seen
		RETURNING id
	`

	var id int64
	err := db.pool.QueryRow(ctx, query,
		entry.IPStart,
		entry.IPEnd,
		entry.CIDR,
		entry.Source,
		entry.SourceName,
		entry.ThreatType,
		entry.Confidence,
		entry.Weight,
		entry.FirstSeen,
		entry.LastSeen,
		entry.ExpiresAt,
	).Scan(&id)

	if err != nil {
		return fmt.Errorf("insert reputation failed: %w", err)
	}

	entry.ID = id
	return nil
}

// InsertReputationBatch inserts multiple reputation entries in a batch
func (db *PostgresDB) InsertReputationBatch(ctx context.Context, entries []IPReputationEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}

	for _, entry := range entries {
		query := `
			INSERT INTO ip_reputation (ip_start, ip_end, cidr, source, source_name, threat_type, confidence, weight, first_seen, last_seen)
			VALUES ($1::inet, $2::inet, $3::cidr, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (ip_start, ip_end, source) 
			DO UPDATE SET
				confidence = GREATEST(ip_reputation.confidence, EXCLUDED.confidence),
				weight = GREATEST(ip_reputation.weight, EXCLUDED.weight),
				last_seen = EXCLUDED.last_seen
		`
		batch.Queue(query,
			entry.IPStart,
			entry.IPEnd,
			entry.CIDR,
			entry.Source,
			entry.SourceName,
			entry.ThreatType,
			entry.Confidence,
			entry.Weight,
			entry.FirstSeen,
			entry.LastSeen,
		)
	}

	results := db.pool.SendBatch(ctx, batch)
	defer results.Close()

	inserted := 0
	for range entries {
		_, err := results.Exec()
		if err != nil {
			// Log but continue
			continue
		}
		inserted++
	}

	return inserted, nil
}

// InsertReputationBulk uses COPY for high-performance bulk insert
func (db *PostgresDB) InsertReputationBulk(ctx context.Context, entries []IPReputationEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	// Create temp table
	_, err := db.pool.Exec(ctx, `
		CREATE TEMP TABLE temp_reputation (
			ip_start INET NOT NULL,
			ip_end INET NOT NULL,
			cidr CIDR,
			source VARCHAR(100) NOT NULL,
			source_name VARCHAR(255),
			threat_type VARCHAR(50) NOT NULL,
			confidence DECIMAL(4,3) NOT NULL,
			weight INTEGER NOT NULL,
			first_seen TIMESTAMP WITH TIME ZONE,
			last_seen TIMESTAMP WITH TIME ZONE
		) ON COMMIT DROP
	`)
	if err != nil {
		return 0, fmt.Errorf("create temp table failed: %w", err)
	}

	// Use COPY to insert into temp table
	columns := []string{"ip_start", "ip_end", "cidr", "source", "source_name", "threat_type", "confidence", "weight", "first_seen", "last_seen"}
	rows := make([][]interface{}, len(entries))

	for i, entry := range entries {
		rows[i] = []interface{}{
			entry.IPStart,
			entry.IPEnd,
			entry.CIDR,
			entry.Source,
			entry.SourceName,
			entry.ThreatType,
			entry.Confidence,
			entry.Weight,
			entry.FirstSeen,
			entry.LastSeen,
		}
	}

	_, err = db.pool.CopyFrom(ctx,
		pgx.Identifier{"temp_reputation"},
		columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("COPY failed: %w", err)
	}

	// Upsert from temp table
	result, err := db.pool.Exec(ctx, `
		INSERT INTO ip_reputation (ip_start, ip_end, cidr, source, source_name, threat_type, confidence, weight, first_seen, last_seen)
		SELECT ip_start, ip_end, cidr, source, source_name, threat_type, confidence, weight, first_seen, last_seen
		FROM temp_reputation
		ON CONFLICT (ip_start, ip_end, source)
		DO UPDATE SET
			confidence = GREATEST(ip_reputation.confidence, EXCLUDED.confidence),
			weight = GREATEST(ip_reputation.weight, EXCLUDED.weight),
			last_seen = EXCLUDED.last_seen
	`)
	if err != nil {
		return 0, fmt.Errorf("upsert failed: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// LookupIP looks up reputation data for an IP
func (db *PostgresDB) LookupIP(ctx context.Context, ip string) ([]IPReputationEntry, error) {
	query := `
		SELECT id, ip_start::text, ip_end::text, cidr::text, source, source_name, threat_type, confidence, weight, first_seen, last_seen
		FROM ip_reputation
		WHERE $1::inet >= ip_start AND $1::inet <= ip_end
		  AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY weight DESC, confidence DESC
	`

	rows, err := db.pool.Query(ctx, query, ip)
	if err != nil {
		return nil, fmt.Errorf("lookup failed: %w", err)
	}
	defer rows.Close()

	var results []IPReputationEntry
	for rows.Next() {
		var entry IPReputationEntry
		err := rows.Scan(
			&entry.ID,
			&entry.IPStart,
			&entry.IPEnd,
			&entry.CIDR,
			&entry.Source,
			&entry.SourceName,
			&entry.ThreatType,
			&entry.Confidence,
			&entry.Weight,
			&entry.FirstSeen,
			&entry.LastSeen,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("Scan error: %v", err))
			continue
		}
		results = append(results, entry)
	}

	return results, nil
}

// IsWhitelisted checks if an IP is whitelisted
func (db *PostgresDB) IsWhitelisted(ctx context.Context, ip string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM whitelist
			WHERE $1::inet >= ip_start AND $1::inet <= ip_end
			  AND (permanent = true OR expires_at IS NULL OR expires_at > NOW())
		)
	`

	var exists bool
	err := db.pool.QueryRow(ctx, query, ip).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("whitelist check failed: %w", err)
	}

	return exists, nil
}

// GetAPIKey retrieves an API key by its hash
func (db *PostgresDB) GetAPIKey(ctx context.Context, keyHash string) (*models.APIKey, error) {
	query := `
		SELECT id, key_hash, name, tier, rate_limit, enabled, created_at, expires_at
		FROM api_keys
		WHERE key_hash = $1
		  AND enabled = true
		  AND (expires_at IS NULL OR expires_at > NOW())
	`

	var key models.APIKey
	var expiresAt *time.Time

	err := db.pool.QueryRow(ctx, query, keyHash).Scan(
		&key.ID,
		&key.Key,
		&key.Name,
		&key.Tier,
		&key.RateLimit,
		&key.Enabled,
		&key.CreatedAt,
		&expiresAt,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get API key failed: %w", err)
	}

	if expiresAt != nil {
		key.ExpiresAt = *expiresAt
	}

	return &key, nil
}

// GetAllActiveReputations fetches all active reputation entries for MMDB compilation
func (db *PostgresDB) GetAllActiveReputations(ctx context.Context) ([]IPReputationEntry, error) {
	query := `
		SELECT id, ip_start::text, ip_end::text, cidr::text, source, source_name, threat_type, confidence, weight, first_seen, last_seen
		FROM ip_reputation
		WHERE (expires_at IS NULL OR expires_at > NOW())
		ORDER BY last_seen DESC
	`

	rows, err := db.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("fetch all reputations failed: %w", err)
	}
	defer rows.Close()

	var results []IPReputationEntry
	for rows.Next() {
		var entry IPReputationEntry
		err := rows.Scan(
			&entry.ID,
			&entry.IPStart,
			&entry.IPEnd,
			&entry.CIDR,
			&entry.Source,
			&entry.SourceName,
			&entry.ThreatType,
			&entry.Confidence,
			&entry.Weight,
			&entry.FirstSeen,
			&entry.LastSeen,
		)
		if err != nil {
			continue
		}
		results = append(results, entry)
	}

	return results, nil
}

// CleanupExpired removes expired entries
func (db *PostgresDB) CleanupExpired(ctx context.Context) (int, error) {
	result, err := db.pool.Exec(ctx, `
		DELETE FROM ip_reputation
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("cleanup failed: %w", err)
	}
	return int(result.RowsAffected()), nil
}

// DBStats holds database statistics
type DBStats struct {
	TotalReputations int64     `json:"total_reputations"`
	TotalSources     int       `json:"total_sources"`
	TotalThreatTypes int       `json:"total_threat_types"`
	WhitelistCount   int64     `json:"whitelist_count"`
	OldestEntry      time.Time `json:"oldest_entry"`
	NewestEntry      time.Time `json:"newest_entry"`
}

// GetStats retrieves database statistics
func (db *PostgresDB) GetStats(ctx context.Context) (*DBStats, error) {
	stats := &DBStats{}

	db.pool.QueryRow(ctx, "SELECT COUNT(*) FROM ip_reputation").Scan(&stats.TotalReputations)
	db.pool.QueryRow(ctx, "SELECT COUNT(DISTINCT source) FROM ip_reputation").Scan(&stats.TotalSources)
	db.pool.QueryRow(ctx, "SELECT COUNT(DISTINCT threat_type) FROM ip_reputation").Scan(&stats.TotalThreatTypes)
	db.pool.QueryRow(ctx, "SELECT COUNT(*) FROM whitelist").Scan(&stats.WhitelistCount)
	db.pool.QueryRow(ctx, "SELECT COALESCE(MIN(first_seen), NOW()), COALESCE(MAX(last_seen), NOW()) FROM ip_reputation").Scan(&stats.OldestEntry, &stats.NewestEntry)

	return stats, nil
}

// Health checks database health
func (db *PostgresDB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.pool.Ping(ctx)
}

// GetASNInfo retrieves ASN information
func (db *PostgresDB) GetASNInfo(ctx context.Context, asn int) (*models.ASNInfo, error) {
	query := `
		SELECT asn, name, org, country_code, asn_type, risk_modifier
		FROM asn_info
		WHERE asn = $1
	`

	var info models.ASNInfo
	err := db.pool.QueryRow(ctx, query, asn).Scan(
		&info.ASN,
		&info.Name,
		&info.Org,
		&info.CountryCode,
		&info.ASNType,
		&info.RiskModifier,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get ASN info failed: %w", err)
	}

	return &info, nil
}

// Helper function to convert IP/Prefix to start/end IP strings
func IPRangeFromPrefix(prefix netip.Prefix) (start, end string) {
	addr := prefix.Addr()
	bits := prefix.Bits()

	if addr.Is4() {
		// IPv4
		maskBits := 32 - bits
		ipBytes := addr.As4()
		startIP := uint32(ipBytes[0])<<24 | uint32(ipBytes[1])<<16 | uint32(ipBytes[2])<<8 | uint32(ipBytes[3])
		startIP = startIP &^ ((1 << maskBits) - 1)
		endIP := startIP | ((1 << maskBits) - 1)

		start = fmt.Sprintf("%d.%d.%d.%d", byte(startIP>>24), byte(startIP>>16), byte(startIP>>8), byte(startIP))
		end = fmt.Sprintf("%d.%d.%d.%d", byte(endIP>>24), byte(endIP>>16), byte(endIP>>8), byte(endIP))
	} else {
		// IPv6 - simplified, just use the prefix masked address
		masked := prefix.Masked()
		start = masked.Addr().String()
		// For IPv6, calculate end is more complex, using a simplified approach
		end = start // For single IP queries this works
	}

	return start, end
}

// IPRangeFromAddr returns start/end for a single IP
func IPRangeFromAddr(addr netip.Addr) (start, end string) {
	s := addr.String()
	return s, s
}
