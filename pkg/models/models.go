package models

import (
	"net/netip"
	"time"
)

// IPReputation represents the reputation data for an IP address
type IPReputation struct {
	ID         int64     `json:"id" db:"id"`
	IPRange    string    `json:"ip_range" db:"ip_range"`
	Source     string    `json:"source" db:"source"`
	ThreatType string    `json:"threat_type" db:"threat_type"`
	Confidence float64   `json:"confidence" db:"confidence"`
	Weight     int       `json:"weight" db:"weight"`
	RiskScore  int       `json:"risk_score" db:"risk_score"`
	FirstSeen  time.Time `json:"first_seen" db:"first_seen"`
	LastSeen   time.Time `json:"last_seen" db:"last_seen"`
	ExpiresAt  time.Time `json:"expires_at,omitempty" db:"expires_at"`
	Metadata   Metadata  `json:"metadata" db:"metadata"`
}

// Metadata holds additional information about an IP
type Metadata struct {
	Country     string   `json:"country,omitempty"`
	City        string   `json:"city,omitempty"`
	ASN         int      `json:"asn,omitempty"`
	Org         string   `json:"org,omitempty"`
	ISP         string   `json:"isp,omitempty"`
	ASNType     string   `json:"asn_type,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`
}

// Threat represents a detected threat for an IP
type Threat struct {
	Type       string    `json:"type"`
	ThreatType string    `json:"threat_type"` // Alias for Type
	Source     string    `json:"source"`
	Confidence float64   `json:"confidence"`
	Weight     int       `json:"weight"`
	LastSeen   time.Time `json:"last_seen"`
}

// ThreatSummary holds aggregated threat information
type ThreatSummary struct {
	TotalThreats  int            `json:"total_threats"`
	ThreatTypes   map[string]int `json:"threat_types"`
	Sources       []string       `json:"sources"`
	MaxConfidence float64        `json:"max_confidence"`
}

// GeoInfo holds geolocation information
type GeoInfo struct {
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
	Region      string  `json:"region,omitempty"`
	City        string  `json:"city,omitempty"`
	PostalCode  string  `json:"postal_code,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
}

// ASNInfo holds ASN information
type ASNInfo struct {
	ASN          int    `json:"asn"`
	Org          string `json:"org"`
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`         // datacenter, isp, business, etc.
	ASNType      string `json:"asn_type,omitempty"`     // Alias for Type
	CountryCode  string `json:"country_code,omitempty"` // Two-letter country code
	Country      string `json:"country,omitempty"`
	RiskModifier int    `json:"risk_modifier,omitempty"` // Bonus/penalty for scoring
}

// IPCheckResult is the result of an IP reputation check
type IPCheckResult struct {
	IP           string   `json:"ip"`
	Score        int      `json:"score"`
	RiskScore    int      `json:"risk_score"` // Alias for Score
	RiskLevel    string   `json:"risk_level"`
	IsProxy      bool     `json:"proxy"`
	IsVPN        bool     `json:"vpn"`
	IsTor        bool     `json:"tor"`
	IsDatacenter bool     `json:"datacenter"`
	IsBotnet     bool     `json:"botnet"`
	IsSpam       bool     `json:"spam"`
	IsMalware    bool     `json:"malware"`
	IsAttacker   bool     `json:"attacker"`
	Threats      []Threat `json:"threats,omitempty"`
	ThreatTypes  []string `json:"threat_types,omitempty"` // List of threat type strings
	Geo          *GeoInfo `json:"geo,omitempty"`
	ASN          *ASNInfo `json:"asn,omitempty"`
	QueryTime    float64  `json:"query_time_ms"`
	Cached       bool     `json:"cached"`
}

// GetRiskLevel returns risk level based on score
func GetRiskLevel(score int) string {
	switch {
	case score >= 85:
		return "critical"
	case score >= 70:
		return "high"
	case score >= 50:
		return "medium"
	case score >= 25:
		return "low"
	default:
		return "safe"
	}
}

// BatchCheckRequest represents a batch IP check request
type BatchCheckRequest struct {
	IPs []string `json:"ips" validate:"required,min=1,max=100"`
}

// BatchCheckResponse represents a batch IP check response
type BatchCheckResponse struct {
	Results    []IPCheckResult `json:"results"`
	TotalTime  float64         `json:"total_time_ms"`
	TotalCount int             `json:"total_count"`
}

// FeedEntry represents an entry from a threat feed
type FeedEntry struct {
	IP         netip.Addr   `json:"-"`
	Prefix     netip.Prefix `json:"-"`
	IPString   string       `json:"ip"`
	Source     string       `json:"source"`
	ThreatType string       `json:"threat_type"`
	Confidence float64      `json:"confidence"`
	Weight     int          `json:"weight"`
	FetchedAt  time.Time    `json:"fetched_at"`
}

// IsPrefix returns true if the entry is a CIDR prefix
func (e *FeedEntry) IsPrefix() bool {
	return e.Prefix.IsValid()
}

// APIKey represents an API key for authentication
type APIKey struct {
	ID        int64     `json:"id" db:"id"`
	Key       string    `json:"key" db:"key"`
	Name      string    `json:"name" db:"name"`
	Tier      string    `json:"tier" db:"tier"` // free, basic, premium, enterprise
	RateLimit int       `json:"rate_limit" db:"rate_limit"`
	Enabled   bool      `json:"enabled" db:"enabled"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// APIStats holds API usage statistics
type APIStats struct {
	TotalRequests   int64   `json:"total_requests"`
	TotalIPs        int64   `json:"total_ips"`
	AvgResponseTime float64 `json:"avg_response_time_ms"`
	ErrorRate       float64 `json:"error_rate"`
	Period          string  `json:"period"`
}

// HealthStatus represents the health status of a service
type HealthStatus struct {
	Status    string            `json:"status"` // healthy, degraded, unhealthy
	Version   string            `json:"version"`
	Uptime    string            `json:"uptime"`
	Timestamp time.Time         `json:"timestamp"`
	Services  map[string]string `json:"services"`
}
