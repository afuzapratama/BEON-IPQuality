package mmdb

import (
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/oschwald/maxminddb-golang"

	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// ReputationRecord represents the data structure stored in the MMDB
type ReputationRecord struct {
	// Risk information
	RiskScore int    `maxminddb:"risk_score"`
	RiskLevel string `maxminddb:"risk_level"`

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
	ThreatType string `maxminddb:"threat_type"`

	// Confidence (0-100 stored as int)
	Confidence int `maxminddb:"confidence"`

	// Source information
	Sources    []string `maxminddb:"sources"`
	LastUpdate int64    `maxminddb:"last_update"` // Unix timestamp

	// Geo information (optional, may be in separate DB)
	Country     string `maxminddb:"country,omitempty"`
	CountryCode string `maxminddb:"country_code,omitempty"`
	City        string `maxminddb:"city,omitempty"`
	Region      string `maxminddb:"region,omitempty"`

	// ASN information (optional)
	ASN     int    `maxminddb:"asn,omitempty"`
	ASNOrg  string `maxminddb:"asn_org,omitempty"`
	ASNType string `maxminddb:"asn_type,omitempty"`
}

// Reader handles reading from the custom MMDB
type Reader struct {
	reputationDB *maxminddb.Reader
	geoipDB      *maxminddb.Reader
	asnDB        *maxminddb.Reader
	mu           sync.RWMutex
}

// NewReader creates a new MMDB reader
func NewReader(reputationPath, geoipPath, asnPath string) (*Reader, error) {
	reader := &Reader{}

	// Load reputation database
	if reputationPath != "" {
		db, err := maxminddb.Open(reputationPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open reputation MMDB: %w", err)
		}
		reader.reputationDB = db
		logger.Info(fmt.Sprintf("Loaded reputation MMDB: %s", reputationPath))
	}

	// Load GeoIP database (optional)
	if geoipPath != "" {
		db, err := maxminddb.Open(geoipPath)
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to open GeoIP MMDB: %v", err))
		} else {
			reader.geoipDB = db
			logger.Info(fmt.Sprintf("Loaded GeoIP MMDB: %s", geoipPath))
		}
	}

	// Load ASN database (optional)
	if asnPath != "" {
		db, err := maxminddb.Open(asnPath)
		if err != nil {
			logger.Warn(fmt.Sprintf("Failed to open ASN MMDB: %v", err))
		} else {
			reader.asnDB = db
			logger.Info(fmt.Sprintf("Loaded ASN MMDB: %s", asnPath))
		}
	}

	return reader, nil
}

// Close closes all open databases
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error

	if r.reputationDB != nil {
		if err := r.reputationDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.geoipDB != nil {
		if err := r.geoipDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if r.asnDB != nil {
		if err := r.asnDB.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing databases: %v", errs)
	}
	return nil
}

// Reload reloads all databases (hot reload)
func (r *Reader) Reload(reputationPath, geoipPath, asnPath string) error {
	// Load new databases first
	newRepDB, err := maxminddb.Open(reputationPath)
	if err != nil {
		return fmt.Errorf("failed to reload reputation MMDB: %w", err)
	}

	var newGeoipDB, newAsnDB *maxminddb.Reader

	if geoipPath != "" {
		newGeoipDB, _ = maxminddb.Open(geoipPath)
	}
	if asnPath != "" {
		newAsnDB, _ = maxminddb.Open(asnPath)
	}

	// Swap databases
	r.mu.Lock()
	oldRepDB := r.reputationDB
	oldGeoipDB := r.geoipDB
	oldAsnDB := r.asnDB

	r.reputationDB = newRepDB
	r.geoipDB = newGeoipDB
	r.asnDB = newAsnDB
	r.mu.Unlock()

	// Close old databases
	if oldRepDB != nil {
		oldRepDB.Close()
	}
	if oldGeoipDB != nil {
		oldGeoipDB.Close()
	}
	if oldAsnDB != nil {
		oldAsnDB.Close()
	}

	logger.Info("Successfully reloaded MMDB databases")
	return nil
}

// LookupReputation looks up reputation data for an IP
func (r *Reader) LookupReputation(ip netip.Addr) (*ReputationRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.reputationDB == nil {
		return nil, fmt.Errorf("reputation database not loaded")
	}

	netIP := net.IP(ip.AsSlice())
	var record ReputationRecord

	err := r.reputationDB.Lookup(netIP, &record)
	if err != nil {
		return nil, err
	}

	// Check if record is empty (IP not found in database)
	if record.RiskScore == 0 && record.ThreatType == "" {
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

	if r.geoipDB == nil {
		return nil, nil // GeoIP not available
	}

	netIP := net.IP(ip.AsSlice())
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

	if r.asnDB == nil {
		return nil, nil // ASN DB not available
	}

	netIP := net.IP(ip.AsSlice())
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
	result := &models.IPCheckResult{
		IP: ip.String(),
	}

	// Lookup reputation
	rep, err := r.LookupReputation(ip)
	if err != nil {
		logger.Debug(fmt.Sprintf("Reputation lookup error for %s: %v", ip, err))
	}
	if rep != nil {
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
		result.ThreatTypes = rep.Sources
		if rep.ThreatType != "" {
			result.ThreatTypes = append([]string{rep.ThreatType}, result.ThreatTypes...)
		}
	} else {
		result.Score = 0
		result.RiskScore = 0
		result.RiskLevel = "clean"
	}

	// Lookup GeoIP
	geo, err := r.LookupGeoIP(ip)
	if err != nil {
		logger.Debug(fmt.Sprintf("GeoIP lookup error for %s: %v", ip, err))
	}
	result.Geo = geo

	// Lookup ASN
	asn, err := r.LookupASN(ip)
	if err != nil {
		logger.Debug(fmt.Sprintf("ASN lookup error for %s: %v", ip, err))
	}
	result.ASN = asn

	return result, nil
}

// Stats returns statistics about the loaded databases
func (r *Reader) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]interface{})

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
