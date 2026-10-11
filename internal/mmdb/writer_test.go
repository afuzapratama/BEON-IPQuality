package mmdb

import (
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

func TestCompileMergesThreatsAndWhitelistOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reputation.mmdb")
	now := time.Now().UTC().Truncate(time.Second)
	entries := []ReputationEntry{
		{
			Prefix: netip.MustParsePrefix("8.8.8.0/24"), RiskScore: 70, RiskLevel: "high",
			ThreatType: "tor", ThreatTypes: []string{"tor"}, Confidence: 1,
			Sources: []string{"tor"}, Flags: EntryFlags{IsTor: true}, LastUpdate: now,
		},
		{
			Prefix: netip.MustParsePrefix("8.8.8.0/24"), RiskScore: 80, RiskLevel: "high",
			ThreatType: "proxy", ThreatTypes: []string{"proxy"}, Confidence: 0.8,
			Sources: []string{"proxy"}, Flags: EntryFlags{IsProxy: true}, LastUpdate: now,
		},
		{
			Prefix: netip.MustParsePrefix("8.8.8.8/32"), RiskLevel: "clean",
			Whitelisted: true, LastUpdate: now,
		},
	}
	if err := NewDefaultWriter().CompileToMMDB(entries, path); err != nil {
		t.Fatal(err)
	}
	reader, err := NewReader(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	merged, err := reader.LookupAll(netip.MustParseAddr("8.8.8.9"))
	if err != nil {
		t.Fatal(err)
	}
	if merged.Score != 100 || !merged.IsTor || !merged.IsProxy || len(merged.Sources) != 2 || len(merged.ThreatTypes) != 2 {
		t.Fatalf("threat records were not merged: %#v", merged)
	}

	allowed, err := reader.LookupAll(netip.MustParseAddr("8.8.8.8"))
	if err != nil {
		t.Fatal(err)
	}
	if !allowed.Whitelisted || allowed.Score != 0 || allowed.IsTor || allowed.IsProxy {
		t.Fatalf("whitelist did not override threat data: %#v", allowed)
	}
}
