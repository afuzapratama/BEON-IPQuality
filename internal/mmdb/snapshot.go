package mmdb

import (
	"crypto/sha256"
	"fmt"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// SnapshotStatus describes the loaded artifact, independently of feed freshness.
type SnapshotStatus struct {
	FeedFreshnessKnown bool                   `json:"feed_freshness_known"`
	Feeds              []models.FeedFreshness `json:"feeds,omitempty"`
	Version            string                 `json:"version"`
	BuiltAt            time.Time              `json:"built_at"`
	AgeSeconds         float64                `json:"age_seconds"`
	Stale              bool                   `json:"stale"`
}

// openSnapshot fingerprints the same immutable file used for a mapped reader.
// Producers must replace files atomically instead of editing them in place.
func openSnapshot(path string) (*maxminddb.Reader, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, "", err
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, "", err
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		db.Close()
		return nil, "", fmt.Errorf("MMDB changed while loading %s; retry reload", path)
	}
	if err := db.Verify(); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("invalid MMDB: %w", err)
	}
	return db, fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// Version includes reputation, geolocation and ASN artifacts used by a lookup.
func (r *Reader) Version() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.version
}

func (r *Reader) Snapshot(maxAge time.Duration, now time.Time) SnapshotStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot(maxAge, now)
}

func (r *Reader) snapshot(maxAge time.Duration, now time.Time) SnapshotStatus {
	status := SnapshotStatus{Version: r.version}
	if r.reputationDB == nil {
		status.Stale = true
		return status
	}
	status.BuiltAt = time.Unix(int64(r.reputationDB.Metadata.BuildEpoch), 0).UTC()
	age := now.Sub(status.BuiltAt)
	status.AgeSeconds = max(0, age.Seconds())
	status.Stale = (maxAge > 0 && age > maxAge) || age < -5*time.Minute
	if r.feedFreshnessKnown {
		status.FeedFreshnessKnown = true
		status.Feeds = append([]models.FeedFreshness(nil), r.feeds...)
		for index := range status.Feeds {
			feed := &status.Feeds[index]
			feed.Stale = feed.LastSuccess == nil || now.Sub(*feed.LastSuccess).Seconds() > feed.MaxAgeSeconds || feed.LastSuccess.After(now.Add(5*time.Minute))
			if feed.Stale {
				status.Stale = true
			}
		}
	}
	return status
}

// VerifyDirectory validates backup/rollback files without requiring PostgreSQL.
func VerifyDirectory(directory string) (*Reader, error) {
	paths := []string{filepath.Join(directory, "GeoLite2-City.mmdb"), filepath.Join(directory, "GeoLite2-ASN.mmdb")}
	for index, path := range paths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			paths[index] = ""
		} else if err != nil {
			return nil, err
		}
	}
	reader, err := NewReader(filepath.Join(directory, "reputation.mmdb"), paths[0], paths[1])
	if err != nil {
		return nil, err
	}
	if (paths[0] != "" && reader.geoipDB == nil) || (paths[1] != "" && reader.asnDB == nil) {
		reader.Close()
		return nil, fmt.Errorf("invalid optional MMDB in snapshot directory")
	}
	return reader, nil
}
