package scoring

import (
	"testing"
	"time"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

func TestConfiguredWeightsDecayCapAndASNBonuses(t *testing.T) {
	cfg := config.ScoringConfig{DecayLambda: 0.5, MaxScore: 60, Weights: map[string]int{"tor_exit_nodes": 20, "tor_exit_nodes/special": 0}, ASNBonuses: map[string]int{"business": 10}}
	feeds := &config.FeedsConfig{Feeds: map[string]config.FeedConfig{"tor_exit_nodes": {Name: "Tor Exit Nodes", Sources: []config.SourceConfig{{Name: "special"}}}}}
	scorer := FromConfig(cfg, feeds)
	now := time.Now()
	threat := models.Threat{Source: "Tor Exit Nodes/torproject", ThreatType: "tor", Weight: 70, Confidence: 0.8, LastSeen: now}
	if score := scorer.CalculateScore([]models.Threat{threat}, nil, now); score != 16 {
		t.Fatalf("configured override ignored: %d", score)
	}
	threat.Source = "Tor Exit Nodes/special"
	if score := scorer.CalculateScore([]models.Threat{threat}, nil, now); score != 0 {
		t.Fatalf("zero override ignored: %d", score)
	}
	threat.Source = "unconfigured"
	threat.Weight = 50
	if score := scorer.CalculateScore([]models.Threat{threat}, &models.ASNInfo{ASNType: "business"}, now); score != 50 {
		t.Fatalf("ASN bonus ignored: %d", score)
	}
	threat.LastSeen = now.Add(-10 * 24 * time.Hour)
	if score := scorer.CalculateScore([]models.Threat{threat}, nil, now); score >= 10 {
		t.Fatalf("decay not applied: %d", score)
	}
	threat.LastSeen = now
	threat.Weight = 95
	if score := scorer.CalculateScore([]models.Threat{threat}, nil, now); score != 60 {
		t.Fatalf("cap not applied: %d", score)
	}
}
