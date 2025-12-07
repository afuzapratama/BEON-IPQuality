package scoring

import (
	"testing"
	"time"

	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

func TestCalculateScore(t *testing.T) {
	scorer := NewDefault()
	now := time.Now()

	tests := []struct {
		name     string
		threats  []models.Threat
		asnInfo  *models.ASNInfo
		minScore int
		maxScore int
	}{
		{
			name:     "No threats",
			threats:  []models.Threat{},
			asnInfo:  nil,
			minScore: 0,
			maxScore: 0,
		},
		{
			name: "Single Tor threat",
			threats: []models.Threat{
				{ThreatType: "tor", Confidence: 1.0, LastSeen: now},
			},
			asnInfo:  nil,
			minScore: 60,
			maxScore: 80,
		},
		{
			name: "Botnet C2 high confidence",
			threats: []models.Threat{
				{ThreatType: "botnet_c2", Confidence: 1.0, LastSeen: now},
			},
			asnInfo:  nil,
			minScore: 90,
			maxScore: 100,
		},
		{
			name: "Multiple threats should increase score",
			threats: []models.Threat{
				{ThreatType: "tor", Confidence: 1.0, LastSeen: now},
				{ThreatType: "proxy", Confidence: 0.8, LastSeen: now},
			},
			asnInfo:  nil,
			minScore: 80,
			maxScore: 100,
		},
		{
			name: "Datacenter ASN adds bonus",
			threats: []models.Threat{
				{ThreatType: "proxy", Confidence: 0.8, LastSeen: now},
			},
			asnInfo:  &models.ASNInfo{ASN: 14061, ASNType: "datacenter"},
			minScore: 50,
			maxScore: 90,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := scorer.CalculateScore(tt.threats, tt.asnInfo, now)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("CalculateScore() = %d, want between %d and %d", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestClassifyRisk(t *testing.T) {
	scorer := NewDefault()

	tests := []struct {
		score int
		want  string
	}{
		{0, "clean"},
		{10, "clean"},
		{24, "clean"},
		{25, "low"},
		{49, "low"},
		{50, "medium"},
		{69, "medium"},
		{70, "high"},
		{84, "high"},
		{85, "critical"},
		{100, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := scorer.ClassifyRisk(tt.score)
			if got != tt.want {
				t.Errorf("ClassifyRisk(%d) = %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}

func TestThreatSummary(t *testing.T) {
	scorer := NewDefault()
	now := time.Now()

	threats := []models.Threat{
		{ThreatType: "tor", Source: "tor_exit", Confidence: 1.0, LastSeen: now},
		{ThreatType: "proxy", Source: "proxy_list", Confidence: 0.8, LastSeen: now},
		{ThreatType: "tor", Source: "tor_exit_2", Confidence: 0.9, LastSeen: now},
	}

	summary := scorer.ThreatSummary(threats)

	if summary.TotalThreats != 3 {
		t.Errorf("TotalThreats = %d, want 3", summary.TotalThreats)
	}

	if summary.ThreatTypes["tor"] != 2 {
		t.Errorf("ThreatTypes[tor] = %d, want 2", summary.ThreatTypes["tor"])
	}

	if summary.ThreatTypes["proxy"] != 1 {
		t.Errorf("ThreatTypes[proxy] = %d, want 1", summary.ThreatTypes["proxy"])
	}

	if summary.MaxConfidence != 1.0 {
		t.Errorf("MaxConfidence = %f, want 1.0", summary.MaxConfidence)
	}
}

func TestGetScoreColor(t *testing.T) {
	scorer := NewDefault()

	tests := []struct {
		score int
		want  string
	}{
		{0, "#28a745"},
		{25, "#17a2b8"},
		{50, "#ffc107"},
		{70, "#fd7e14"},
		{85, "#dc3545"},
	}

	for _, tt := range tests {
		got := scorer.GetScoreColor(tt.score)
		if got != tt.want {
			t.Errorf("GetScoreColor(%d) = %v, want %v", tt.score, got, tt.want)
		}
	}
}

// Benchmark tests
func BenchmarkCalculateScore(b *testing.B) {
	scorer := NewDefault()
	now := time.Now()
	threats := []models.Threat{
		{ThreatType: "tor", Confidence: 1.0, LastSeen: now},
		{ThreatType: "proxy", Confidence: 0.8, LastSeen: now.Add(-24 * time.Hour)},
	}
	asnInfo := &models.ASNInfo{ASN: 14061, ASNType: "datacenter"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.CalculateScore(threats, asnInfo, now)
	}
}

func BenchmarkClassifyRisk(b *testing.B) {
	scorer := NewDefault()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.ClassifyRisk(75)
	}
}
