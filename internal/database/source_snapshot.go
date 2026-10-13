package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReplaceReputationSource publishes a complete source in one transaction.
// Removed entries expire immediately; failures leave the previous source intact.
func (db *PostgresDB) ReplaceReputationSource(ctx context.Context, source string, entries []IPReputationEntry, expiry time.Time) (int, error) {
	if len(entries) == 0 {
		return 0, fmt.Errorf("refusing empty source snapshot")
	}
	for _, entry := range entries {
		if entry.Source != source {
			return 0, fmt.Errorf("source snapshot contains mixed sources")
		}
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", source); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE beon_feed_stage (ip_start TEXT, ip_end TEXT, cidr TEXT, threat_type TEXT, confidence DOUBLE PRECISION, weight INTEGER) ON COMMIT DROP`); err != nil {
		return 0, err
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"beon_feed_stage"}, []string{"ip_start", "ip_end", "cidr", "threat_type", "confidence", "weight"}, pgx.CopyFromSlice(len(entries), func(index int) ([]any, error) {
		entry := entries[index]
		return []any{entry.IPStart, entry.IPEnd, entry.CIDR, entry.ThreatType, entry.Confidence, entry.Weight}, nil
	}))
	if err != nil {
		return 0, fmt.Errorf("stage source: %w", err)
	}
	// The expiry anti-join must not repeatedly scan a million-row staging table.
	// Expression keys match the inet casts used by the publication query.
	if _, err := tx.Exec(ctx, `CREATE INDEX beon_feed_stage_ips ON beon_feed_stage ((ip_start::inet),(ip_end::inet))`); err != nil {
		return 0, fmt.Errorf("index staged source: %w", err)
	}
	if _, err := tx.Exec(ctx, `ANALYZE beon_feed_stage`); err != nil {
		return 0, fmt.Errorf("analyze staged source: %w", err)
	}
	result, err := tx.Exec(ctx, `INSERT INTO ip_reputation (ip_start,ip_end,cidr,source,threat_type,confidence,weight,first_seen,last_seen,expires_at)
 SELECT DISTINCT ON (ip_start,ip_end) ip_start::inet,ip_end::inet,cidr::cidr,$1,threat_type,confidence,weight,NOW(),NOW(),$2 FROM beon_feed_stage ORDER BY ip_start,ip_end
 ON CONFLICT (ip_start,ip_end,source) DO UPDATE SET cidr=EXCLUDED.cidr,threat_type=EXCLUDED.threat_type,confidence=EXCLUDED.confidence,weight=EXCLUDED.weight,last_seen=EXCLUDED.last_seen,expires_at=EXCLUDED.expires_at`, source, expiry)
	if err != nil {
		return 0, fmt.Errorf("publish source: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE ip_reputation r SET expires_at=NOW() WHERE source=$1 AND (expires_at IS NULL OR expires_at>NOW()) AND NOT EXISTS (SELECT 1 FROM beon_feed_stage s WHERE s.ip_start::inet=r.ip_start AND s.ip_end::inet=r.ip_end)`, source); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int(result.RowsAffected()), nil
}

// PruneHistory keeps the last attempt and successful fetch per source even when
// those records are older than the general retention window.
func (db *PostgresDB) PruneHistory(ctx context.Context, reputationRetention, historyRetention time.Duration) error {
	if reputationRetention <= 0 || historyRetention <= 0 {
		return fmt.Errorf("retention windows must be positive")
	}
	if _, err := db.pool.Exec(ctx, `DELETE FROM ip_reputation WHERE expires_at < NOW()-($1 * INTERVAL '1 second')`, reputationRetention.Seconds()); err != nil {
		return err
	}
	_, err := db.pool.Exec(ctx, `DELETE FROM feed_fetch_history WHERE created_at < NOW()-($1 * INTERVAL '1 second') AND id NOT IN (SELECT max(id) FROM feed_fetch_history GROUP BY feed_name,source_url) AND id NOT IN (SELECT max(id) FROM feed_fetch_history WHERE status='success' GROUP BY feed_name,source_url)`, historyRetention.Seconds())
	return err
}
