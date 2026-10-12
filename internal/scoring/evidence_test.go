package scoring

import (
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
	"testing"
	"time"
)

func TestCorrelatedSignalsDoNotInflateRisk(t *testing.T) {
	s := NewDefault()
	now := time.Now()
	one := models.Threat{ThreatType: "proxy", Source: "original", Weight: 60, Confidence: 1, LastSeen: now}
	threats := []models.Threat{one}
	original := s.CalculateScore(threats, nil, now)
	for i := 0; i < 100; i++ {
		copy := one
		copy.Source = "aggregator"
		threats = append(threats, copy)
	}
	if got := s.CalculateScore(threats, nil, now); got != original {
		t.Fatalf("duplicate risk %d != %d", got, original)
	}
}

func TestInfrastructureAloneDoesNotFlagMalicious(t *testing.T) {
	s := NewDefault()
	now := time.Now()
	threats := []models.Threat{{ThreatType: "datacenter", Weight: 95, Confidence: 1, LastSeen: now}, {ThreatType: "hosting", Weight: 80, Confidence: 1, LastSeen: now}}
	if got := s.CalculateScore(threats, &models.ASNInfo{ASNType: "datacenter"}, now); got != 20 {
		t.Fatalf("infrastructure score %d, want 20", got)
	}
	threats = append(threats, models.Threat{ThreatType: "botnet_c2", Weight: 95, Confidence: 1, LastSeen: now})
	if got := s.CalculateScore(threats, &models.ASNInfo{ASNType: "government"}, now); got < 95 {
		t.Fatalf("ASN masked C2: %d", got)
	}
}

func TestEvidenceCombinesIndependentFamiliesAndDecay(t *testing.T) {
	s := NewDefault()
	now := time.Now()
	threat := models.Threat{ThreatType: "proxy", Weight: 60, Confidence: 1, LastSeen: now}
	fresh := s.CalculateScore([]models.Threat{threat}, nil, now)
	threat.LastSeen = now.Add(-90 * 24 * time.Hour)
	if old := s.CalculateScore([]models.Threat{threat}, nil, now); old >= fresh {
		t.Fatalf("decay %d >= %d", old, fresh)
	}
	if got := ScoreEvidence(map[string]int{"anonymity": 7000, "abuse": 9000}, 0, 100); got != 97 {
		t.Fatalf("combined score %d", got)
	}
}
