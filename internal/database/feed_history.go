package database

import (
	"context"
	"time"
)

// RecordFeedFetch records successful fetch/storage
// separately from failures. The compiler carries these timestamps into MMDB.
func (db *PostgresDB) RecordFeedFetch(ctx context.Context, feed, sourceURL, status string, entries, stored int, duration time.Duration, message string) error {
	_, err := db.pool.Exec(ctx, `INSERT INTO feed_fetch_history (feed_name,source_url,status,entries_count,updated_entries,duration_ms,error_message) VALUES ($1,$2,$3,$4,$5,$6,$7)`, feed, sourceURL, status, entries, stored, duration.Milliseconds(), message)
	return err
}
