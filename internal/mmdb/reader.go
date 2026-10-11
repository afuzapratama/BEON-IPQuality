package mmdb

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"

	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// ReputationRecord represents the data structure stored in the MMDB
type ReputationRecord struct {
	// Risk information
	RiskScore     int    `maxminddb:"risk_score"`
	RiskLevel     string `maxminddb:"risk_level"`
	RiskThreshold int    `maxminddb:"risk_threshold"`

	// Threat information
	IsTor        bool `maxminddb:"is_tor"`
	IsVPN        bool `maxminddb:"is_vpn"`
	IsProxy      bool `maxminddb:"is_proxy"`
	IsDatacenter bool `maxminddb:"is_datacenter"`
	IsBotnet     bool `maxminddb:"is_botnet"`
	IsMalware    bool `maxminddb:"is_malware"`
	IsSpam       bool `maxminddb:"is_spam"`
	IsAttacker   bool `maxminddb:"is_attacker"`

	// Primary threat type
	ThreatType  string   `maxminddb:"threat_type"`
	ThreatTypes []string `maxminddb:"threat_types"`

	// Confidence (0-100 stored as int)
	Confidence int `maxminddb:"confidence"`

	// Source information
	Sources     []string `maxminddb:"sources"`
	LastUpdate  int64    `maxminddb:"last_update"` // Unix timestamp
	Whitelisted bool     `maxminddb:"whitelisted"`

	// Geo information (optional, may be in separate DB)
	Country     string `maxminddb:"country"`
	CountryCode string `maxminddb:"country_code"`
	City        string `maxminddb:"city"`
	Region      string `maxminddb:"region"`

	// ASN information (optional)
	ASN     int    `maxminddb:"asn"`
	ASNOrg  string `maxminddb:"asn_org"`
	ASNType string `maxminddb:"asn_type"`
}

// Reader handles reading from the custom MMDB
type Reader struct {
	feeds              []models.FeedFreshness
	feedFreshnessKnown bool
	reputationDB       *maxminddb.Reader
	geoipDB            *maxminddb.Reader
	asnDB              *maxminddb.Reader
	mu                 sync.RWMutex
	reloadMu           sync.Mutex
	version            string
	closed             bool
	repVersion         string
	geoVersion         string
	asnVersion         string
}

// NewReader creates a new MMDB reader
func NewReader(reputationPath, geoipPath, asnPath string) (*Reader, error) {
	reader := &Reader{}

	// Load reputation database
	if reputationPath != "" {
		db, version, err := openSnapshot(reputationPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open reputation MMDB: %w", err)
		}
		if db.Metadata.DatabaseType != "BEON-IPReputation" {
			db.Close()
			return nil, fmt.Errorf("unexpected reputation database type: %s", db.Metadata.DatabaseType)
		}
		reader.reputationDB = db
		reader.repVersion = version
		if raw, ok := db.Metadata.Description["beon_feed_freshness"]; ok {
			if err := json.Unmarshal([]byte(raw), &reader.feeds); err != nil {
				db.Close()
				return nil, fmt.Errorf("invalid feed freshness metadata: %w", err)
			}
			reader.feedFreshnessKnown = true
		}
		logger.Info(fmt.Sprintf("Loaded reputation MMDB: %s", reputationPath))
	}

	// Load GeoIP database (optional)
	if geoipPath != "" {
		db, version, err := openSnapshot(geoipPath)
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to open GeoIP MMDB: %v", err))
		} else {
			reader.geoipDB = db
			reader.geoVersion = version
			logger.Info(fmt.Sprintf("Loaded GeoIP MMDB: %s", geoipPath))
		}
	}

	// Load ASN database (optional)
	if asnPath != "" {
		db, version, err := openSnapshot(asnPath)
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to open ASN MMDB: %v", err))
		} else {
			reader.asnDB = db
			reader.asnVersion = version
			logger.Info(fmt.Sprintf("Loaded ASN MMDB: %s", asnPath))
		}
	}

	reader.version = fmt.Sprintf("%x", sha256.Sum256([]byte(reader.repVersion+":"+reader.geoVersion+":"+reader.asnVersion)))
	return reader, nil
}

// Close closes all open databases
func (r *Reader) Close() error {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true
	var errs []error

	if r.reputationDB != nil {
		if err := r.reputationDB.Close(); err != nil {
			errs = append(errs, err)
		}
		r.reputationDB = nil
	}
	if r.geoipDB != nil {
		if err := r.geoipDB.Close(); err != nil {
			errs = append(errs, err)
		}
		r.geoipDB = nil
	}
	if r.asnDB != nil {
		if err := r.asnDB.Close(); err != nil {
			errs = append(errs, err)
		}
		r.asnDB = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}
	return nil
}

// IsReady reports whether reputation lookups can be served safely.
func (r *Reader) IsReady() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.reputationDB != nil
}

// Reload reloads all databases (hot reload)
func (r *Reader) Reload(reputationPath, geoipPath, asnPath string) error {
	_, err := r.ReloadIfChanged(reputationPath, geoipPath, asnPath)
	return err
}

// ReloadIfChanged swaps the complete lookup generation and keeps loaded optional
// databases when a requested replacement cannot be opened.
func (r *Reader) ReloadIfChanged(reputationPath, geoipPath, asnPath string) (bool, error) {
	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()
	candidate, err := NewReader(reputationPath, geoipPath, asnPath)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		candidate.Close()
		return false, fmt.Errorf("reader is closed")
	}
	if candidate.reputationDB == nil || (geoipPath != "" && r.geoipDB != nil && candidate.geoipDB == nil) || (asnPath != "" && r.asnDB != nil && candidate.asnDB == nil) {
		r.mu.Unlock()
		candidate.Close()
		return false, fmt.Errorf("reload refused: requested database unavailable")
	}
	if r.version == candidate.version {
		r.mu.Unlock()
		candidate.Close()
		return false, nil
	}
	oldRep, oldGeo, oldASN := r.reputationDB, r.geoipDB, r.asnDB
	r.reputationDB, r.geoipDB, r.asnDB = candidate.reputationDB, candidate.geoipDB, candidate.asnDB
	r.version, r.repVersion, r.geoVersion, r.asnVersion = candidate.version, candidate.repVersion, candidate.geoVersion, candidate.asnVersion
	r.feeds, r.feedFreshnessKnown = candidate.feeds, candidate.feedFreshnessKnown
	// All previous readers finished before this write lock; new requests see the
	// complete new generation. Closing old mappings while locked is safe.
	if oldRep != nil {
		oldRep.Close()
	}
	if oldGeo != nil {
		oldGeo.Close()
	}
	if oldASN != nil {
		oldASN.Close()
	}
	r.mu.Unlock()
	logger.Info("Successfully reloaded changed MMDB generation")
	return true, nil
}

// LookupReputation looks up reputation data for an IP
func (r *Reader) LookupReputation(ip netip.Addr) (*ReputationRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupReputation(ip)
}

func (r *Reader) lookupReputation(ip netip.Addr) (*ReputationRecord, error) {

	if r.reputationDB == nil {
		return nil, fmt.Errorf("reputation database not loaded")
	}

	// Unmap IPv4-in-IPv6 (e.g. ::ffff:1.2.3.4 → 1.2.3.4) so MaxMind
	// receives the canonical 4-byte representation for IPv4 databases.
	netIP := net.IP(ip.Unmap().AsSlice())
	var record ReputationRecord

	err := r.reputationDB.Lookup(netIP, &record)
	if err != nil {
		return nil, err
	}

	// Check if record is empty (IP not found in database)
	if record.RiskScore == 0 && record.ThreatType == "" && !record.Whitelisted {
		return nil, nil // Not found, but not an error
	}

	return &record, nil
}

// GeoIPRecord represents GeoIP lookup result
type GeoIPRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
	Subdivisions []struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"subdivisions"`
	Location struct {
		Latitude  float64 `maxminddb:"latitude"`
		Longitude float64 `maxminddb:"longitude"`
		TimeZone  string  `maxminddb:"time_zone"`
	} `maxminddb:"location"`
}

// LookupGeoIP looks up geo information for an IP
func (r *Reader) LookupGeoIP(ip netip.Addr) (*models.GeoInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupGeoIP(ip)
}

func (r *Reader) lookupGeoIP(ip netip.Addr) (*models.GeoInfo, error) {

	if r.geoipDB == nil {
		return nil, nil // GeoIP not available
	}

	netIP := net.IP(ip.Unmap().AsSlice())
	var record GeoIPRecord

	err := r.geoipDB.Lookup(netIP, &record)
	if err != nil {
		return nil, err
	}

	geo := &models.GeoInfo{
		CountryCode: record.Country.ISOCode,
		Latitude:    record.Location.Latitude,
		Longitude:   record.Location.Longitude,
		Timezone:    record.Location.TimeZone,
	}

	if name, ok := record.Country.Names["en"]; ok {
		geo.Country = name
	}
	if name, ok := record.City.Names["en"]; ok {
		geo.City = name
	}
	if len(record.Subdivisions) > 0 {
		if name, ok := record.Subdivisions[0].Names["en"]; ok {
			geo.Region = name
		}
	}

	return geo, nil
}

// ASNRecord represents ASN lookup result
type ASNRecord struct {
	AutonomousSystemNumber       uint   `maxminddb:"autonomous_system_number"`
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
}

// LookupASN looks up ASN information for an IP
func (r *Reader) LookupASN(ip netip.Addr) (*models.ASNInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupASN(ip)
}

func (r *Reader) lookupASN(ip netip.Addr) (*models.ASNInfo, error) {

	if r.asnDB == nil {
		return nil, nil // ASN DB not available
	}

	netIP := net.IP(ip.Unmap().AsSlice())
	var record ASNRecord

	err := r.asnDB.Lookup(netIP, &record)
	if err != nil {
		return nil, err
	}

	return &models.ASNInfo{
		ASN: int(record.AutonomousSystemNumber),
		Org: record.AutonomousSystemOrganization,
	}, nil
}

// LookupAll performs a complete lookup for an IP
func (r *Reader) LookupAll(ip netip.Addr) (*models.IPCheckResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lookupAll(ip)
}

// LookupAllFresh checks freshness and looks up data under the same generation lock.
func (r *Reader) LookupAllFresh(ip netip.Addr, maxAge time.Duration, now time.Time) (*models.IPCheckResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.snapshot(maxAge, now).Stale {
		return nil, fmt.Errorf("snapshot or feed is stale")
	}
	return r.lookupAll(ip)
}

func (r *Reader) lookupAll(ip netip.Addr) (*models.IPCheckResult, error) {
	result := &models.IPCheckResult{
		IP:          ip.String(),
		DataVersion: r.version,
	}

	// Lookup reputation
	rep, err := r.lookupReputation(ip)
	if err != nil {
		return nil, fmt.Errorf("reputation lookup for %s failed: %w", ip, err)
	}
	if rep != nil {
		if rep.Whitelisted {
			result.Score = 0
			result.RiskScore = 0
			result.RiskLevel = "clean"
			result.Whitelisted = true
		} else {
			threshold := rep.RiskThreshold
			if threshold <= 0 {
				threshold = 50
			}
			result.Risky = rep.RiskScore >= threshold
			result.Score = rep.RiskScore
			result.RiskScore = rep.RiskScore
			result.RiskLevel = rep.RiskLevel
			result.IsTor = rep.IsTor
			result.IsVPN = rep.IsVPN
			result.IsProxy = rep.IsProxy
			result.IsDatacenter = rep.IsDatacenter
			result.IsBotnet = rep.IsBotnet
			result.IsMalware = rep.IsMalware
			result.IsSpam = rep.IsSpam
			result.IsAttacker = rep.IsAttacker
			result.ThreatTypes = rep.ThreatTypes
			result.Sources = rep.Sources
			if len(result.ThreatTypes) == 0 && rep.ThreatType != "" {
				result.ThreatTypes = []string{rep.ThreatType}
			}
		}
	} else {
		result.Score = 0
		result.RiskScore = 0
		result.RiskLevel = "clean"
	}

	// Lookup GeoIP
	geo, err := r.lookupGeoIP(ip)
	if err != nil {
		logger.Debug(fmt.Sprintf("GeoIP lookup error for %s: %v", ip, err))
	}
	result.Geo = geo

	// Lookup ASN
	asn, err := r.lookupASN(ip)
	if err != nil {
		logger.Debug(fmt.Sprintf("ASN lookup error for %s: %v", ip, err))
	}
	if asn != nil && rep != nil && rep.ASN == asn.ASN {
		asn.ASNType = rep.ASNType
		asn.Type = rep.ASNType
	}
	result.ASN = asn

	return result, nil
}

// Stats returns statistics about the loaded databases
func (r *Reader) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["version"] = r.version

	if r.reputationDB != nil {
		meta := r.reputationDB.Metadata
		stats["reputation"] = map[string]interface{}{
			"build_epoch":   meta.BuildEpoch,
			"database_type": meta.DatabaseType,
			"ip_version":    meta.IPVersion,
			"node_count":    meta.NodeCount,
			"record_size":   meta.RecordSize,
			"binary_format": meta.BinaryFormatMajorVersion,
		}
	}

	if r.geoipDB != nil {
		meta := r.geoipDB.Metadata
		stats["geoip"] = map[string]interface{}{
			"database_type": meta.DatabaseType,
			"build_epoch":   meta.BuildEpoch,
		}
	}

	if r.asnDB != nil {
		meta := r.asnDB.Metadata
		stats["asn"] = map[string]interface{}{
			"database_type": meta.DatabaseType,
			"build_epoch":   meta.BuildEpoch,
		}
	}

	return stats
}

// LookupASNPrefix annotates a range only when one ASN record covers it entirely.
func (r *Reader) LookupASNPrefix(prefix netip.Prefix) (*models.ASNInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.asnDB == nil {
		return nil, nil
	}
	var record ASNRecord
	network, found, err := r.asnDB.LookupNetwork(net.IP(prefix.Masked().Addr().AsSlice()), &record)
	if err != nil || !found {
		return nil, err
	}
	asnPrefix, err := netip.ParsePrefix(network.String())
	if err != nil {
		return nil, err
	}
	if asnPrefix.Bits() > prefix.Bits() || !asnPrefix.Contains(prefix.Addr()) {
		return nil, nil
	}
	return &models.ASNInfo{ASN: int(record.AutonomousSystemNumber), Org: record.AutonomousSystemOrganization}, nil
}

func (r *Reader) HasASN() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.asnDB != nil
}
