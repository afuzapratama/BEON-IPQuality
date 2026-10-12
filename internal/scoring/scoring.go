package scoring

import (
	"math"
	"time"

	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// Config holds scoring configuration
type Config struct {
	SourceWeights map[string]int
	// Base weights for each threat type
	ThreatWeights map[string]int

	// ASN type risk modifiers
	ASNTypeModifiers map[string]int

	// Time decay parameters
	DecayLambda float64 // Decay rate (higher = faster decay)
	MaxAge      time.Duration

	// Score bounds
	MinScore int
	MaxScore int
}

// DefaultConfig returns the default scoring configuration
func DefaultConfig() Config {
	return Config{
		ThreatWeights: map[string]int{
			"tor":        70,
			"vpn":        45,
			"proxy":      50,
			"datacenter": 40,
			"botnet_c2":  95,
			"botnet":     90,
			"malware":    90,
			"malicious":  85,
			"spam":       60,
			"hijacked":   95,
			"attack":     75,
			"suspicious": 55,
			"anonymous":  50, // firehol_anonymous: anonymous proxies/VPNs
			"hosting":    35,
			"bruteforce": 60, // brute force attackers (blocklist.de, rulez.sk)
		},
		ASNTypeModifiers: map[string]int{
			"datacenter": 15,
			"hosting":    15,
			"isp":        0,
			"business":   -10,
			"education":  -20,
			"government": -25,
		},
		DecayLambda: 0.01,                 // ~70 day half-life
		MaxAge:      180 * 24 * time.Hour, // 180 days
		MinScore:    0,
		MaxScore:    100,
	}
}

// Scorer calculates risk scores for IPs
type Scorer struct {
	config Config
}

// New creates a new Scorer with the given configuration
func New(config Config) *Scorer {
	return &Scorer{config: config}
}

// NewDefault creates a new Scorer with default configuration
func NewDefault() *Scorer {
	return New(DefaultConfig())
}

// CalculateScore applies the versioned conservative evidence policy.
func (s *Scorer) CalculateScore(threats []models.Threat, asnInfo *models.ASNInfo, now time.Time) int {
	return ScoreEvidence(s.Evidence(threats, asnInfo, now), s.config.MinScore, s.config.MaxScore)
}

// calculateDecay calculates the time decay factor
// D(t) = e^(-λt) where t is time since last seen in days
func (s *Scorer) calculateDecay(lastSeen, now time.Time) float64 {
	if lastSeen.IsZero() {
		return 0.5 // Default for unknown last seen
	}

	age := now.Sub(lastSeen)

	// If too old, return minimum decay
	if age > s.config.MaxAge {
		return 0.1
	}

	// If recently seen, no decay
	if age < 24*time.Hour {
		return 1.0
	}

	// Calculate exponential decay
	days := age.Hours() / 24
	decay := math.Exp(-s.config.DecayLambda * days)

	// Ensure minimum decay factor
	if decay < 0.1 {
		decay = 0.1
	}

	return decay
}

// getThreatWeight returns the weight for a threat type
func (s *Scorer) getThreatWeight(threatType string) int {
	if weight, ok := s.config.ThreatWeights[threatType]; ok {
		return weight
	}
	return 50 // Default weight
}

// getASNModifier returns the risk modifier for an ASN type
func (s *Scorer) getASNModifier(asnType string) int {
	if modifier, ok := s.config.ASNTypeModifiers[asnType]; ok {
		return modifier
	}
	return 0
}

// ClassifyRisk classifies the risk level based on score
func (s *Scorer) ClassifyRisk(score int) string {
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
		return "clean"
	}
}

// GetScoreColor returns a color for visualization (hex color)
func (s *Scorer) GetScoreColor(score int) string {
	switch {
	case score >= 85:
		return "#dc3545" // Red
	case score >= 70:
		return "#fd7e14" // Orange
	case score >= 50:
		return "#ffc107" // Yellow
	case score >= 25:
		return "#17a2b8" // Cyan
	default:
		return "#28a745" // Green
	}
}

// ThreatSummary generates a summary of detected threats
func (s *Scorer) ThreatSummary(threats []models.Threat) models.ThreatSummary {
	summary := models.ThreatSummary{
		TotalThreats: len(threats),
		ThreatTypes:  make(map[string]int),
		Sources:      make([]string, 0),
	}

	sourceMap := make(map[string]bool)
	var maxConfidence float64

	for _, threat := range threats {
		summary.ThreatTypes[threat.ThreatType]++
		if !sourceMap[threat.Source] {
			sourceMap[threat.Source] = true
			summary.Sources = append(summary.Sources, threat.Source)
		}
		if threat.Confidence > maxConfidence {
			maxConfidence = threat.Confidence
		}
	}

	summary.MaxConfidence = maxConfidence

	return summary
}

// ScoringResult holds the complete scoring result
type ScoringResult struct {
	Score         int
	RiskLevel     string
	Color         string
	ThreatSummary models.ThreatSummary
	DecayApplied  bool
	Multipliers   []string
}

// CalculateDetailedScore returns a detailed scoring result
func (s *Scorer) CalculateDetailedScore(threats []models.Threat, asnInfo *models.ASNInfo, now time.Time) ScoringResult {
	score := s.CalculateScore(threats, asnInfo, now)

	result := ScoringResult{
		Score:         score,
		RiskLevel:     s.ClassifyRisk(score),
		Color:         s.GetScoreColor(score),
		ThreatSummary: s.ThreatSummary(threats),
		Multipliers:   make([]string, 0),
	}

	// Check what multipliers were applied
	threatTypes := make(map[string]bool)
	for _, threat := range threats {
		threatTypes[threat.ThreatType] = true
		if !threat.LastSeen.IsZero() && now.Sub(threat.LastSeen) > 24*time.Hour {
			result.DecayApplied = true
		}
	}

	if len(threatTypes) > 1 {
		result.Multipliers = append(result.Multipliers, "multi_threat")
	}

	if asnInfo != nil && (asnInfo.ASNType == "datacenter" || asnInfo.ASNType == "hosting") {
		result.Multipliers = append(result.Multipliers, "datacenter")
	}

	return result
}
