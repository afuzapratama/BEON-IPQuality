package compiler

import (
	"bytes"
	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"net/netip"
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
	if entry.RiskScore != 70 {
		t.Fatalf("combined risk score = %d, want strongest correlated anonymity signal", entry.RiskScore)
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

func TestCompilerAcceptsReservedFeedNetworksAndLocalWhitelist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reputation.mmdb")
	entries := []mmdb.ReputationEntry{
		{Prefix: netip.MustParsePrefix("0.0.0.0/32"), RiskScore: 85, RiskLevel: "high"},
		{Prefix: netip.MustParsePrefix("127.0.0.0/8"), RiskScore: 85, RiskLevel: "high"},
		{Prefix: netip.MustParsePrefix("127.0.0.0/8"), Whitelisted: true, RiskLevel: "clean"},
		{Prefix: netip.MustParsePrefix("10.0.0.0/8"), Whitelisted: true, RiskLevel: "clean"},
		{Prefix: netip.MustParsePrefix("::1/128"), Whitelisted: true, RiskLevel: "clean"},
	}
	if err := mmdb.NewWriter(reputationWriterConfig(28)).CompileToMMDB(entries, path); err != nil {
		t.Fatal(err)
	}
	reader, err := mmdb.NewReader(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "::1"} {
		got, err := reader.LookupAll(netip.MustParseAddr(ip))
		if err != nil || !got.Whitelisted || got.Score != 0 {
			t.Fatalf("%s: whitelist missing: %#v, %v", ip, got, err)
		}
	}
}

func TestEnabledSourcesExcludeDisabledAndRemovedSources(t *testing.T) {
	feeds := &config.FeedsConfig{Feeds: map[string]config.FeedConfig{
		"active":   {Enabled: true, Name: "Active Feed", Sources: []config.SourceConfig{{Name: "current"}}},
		"disabled": {Enabled: false, Name: "Disabled Feed", Sources: []config.SourceConfig{{Name: "old"}}},
	}}
	ids := enabledSourceIDs(feeds)
	if len(ids) != 2 || ids[0] != "Active Feed/current" || ids[1] != "active/current" {
		t.Fatalf("source IDs: %v", ids)
	}
}

func TestProductionWriterAcceptsExplicitSixToFourRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "six-to-four.mmdb")
	entries := []mmdb.ReputationEntry{
		{Prefix: netip.MustParsePrefix("168.144.132.25/32"), RiskScore: 80},
		{Prefix: netip.MustParsePrefix("2002:a890:8419::a890:8419/128"), RiskScore: 32},
	}
	if err := mmdb.NewWriter(reputationWriterConfig(28)).CompileToMMDB(entries, path); err != nil {
		t.Fatal(err)
	}
	reader, err := mmdb.NewReader(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for ip, want := range map[string]int{"168.144.132.25": 80, "2002:a890:8419::a890:8419": 32} {
		got, err := reader.LookupAll(netip.MustParseAddr(ip))
		if err != nil || got.Score != want {
			t.Fatalf("%s: %#v %v", ip, got, err)
		}
	}
}
