package compiler

import (
	"bytes"
	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lfrfrfr/beon-ipquality/internal/scoring"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

func TestAggregateReputationsCombinesSourcesAndThreats(t *testing.T) {
	now := time.Now().UTC()
	compiler := &Compiler{scorer: scoring.NewDefault()}
	entries, err := compiler.aggregateReputations([]models.IPReputation{
		{IPRange: "8.8.8.0/24", Source: "tor/source", ThreatType: "tor", Confidence: 1, Weight: 70, LastSeen: now},
		{IPRange: "8.8.8.0/24", Source: "proxy/source", ThreatType: "proxy", Confidence: 0.8, Weight: 40, LastSeen: now},
		{IPRange: "not-an-ip", Source: "bad", ThreatType: "spam", Confidence: 1, Weight: 100, LastSeen: now},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	entry := entries[0]
	if len(entry.Sources) != 2 || len(entry.ThreatTypes) != 2 || !entry.Flags.IsTor || !entry.Flags.IsProxy {
		t.Fatalf("signals were not aggregated: %#v", entry)
	}
	if entry.RiskScore <= 70 {
		t.Fatalf("combined risk score = %d, want greater than single-source score", entry.RiskScore)
	}
}

func TestEmptyCompilationPreservesExistingSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reputation.mmdb")
	original := []byte("previous snapshot")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.MMDB.OutputPath = path
	c := &Compiler{config: cfg}
	if err := c.publishEntries(nil); err == nil {
		t.Fatal("empty compilation must fail")
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("previous snapshot changed: %v", err)
	}
	cfg.MMDB.OutputPath = filepath.Join(t.TempDir(), "missing.mmdb")
	if err := c.publishEntries(nil); err == nil {
		t.Fatal("empty bootstrap must fail")
	}
	if _, err := os.Stat(cfg.MMDB.OutputPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected bootstrap artifact: %v", err)
	}
}
