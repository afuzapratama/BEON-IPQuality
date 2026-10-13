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
	if merged.Score != 80 || !merged.IsTor || !merged.IsProxy || len(merged.Sources) != 2 || len(merged.ThreatTypes) != 2 {
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

func TestEvidenceOverlapIsOrderIndependentAndRetainsCap(t *testing.T) {
	now := time.Now()
	entries := []ReputationEntry{
		{Prefix: netip.MustParsePrefix("8.8.8.0/24"), RiskScore: 60, RiskLevel: "medium", Sources: []string{"proxy-a"}, ThreatType: "proxy", ThreatTypes: []string{"proxy"}, Evidence: map[string]int{"anonymity": 6000}, ScoreCap: 75, LastUpdate: now},
		{Prefix: netip.MustParsePrefix("8.8.8.0/25"), RiskScore: 60, RiskLevel: "medium", Sources: []string{"proxy-b"}, ThreatType: "proxy", ThreatTypes: []string{"proxy"}, Evidence: map[string]int{"anonymity": 6000}, ScoreCap: 75, LastUpdate: now},
		{Prefix: netip.MustParsePrefix("8.8.8.0/26"), RiskScore: 70, RiskLevel: "high", Sources: []string{"malware"}, ThreatType: "malware", ThreatTypes: []string{"malware"}, Evidence: map[string]int{"abuse": 7000}, ScoreCap: 75, LastUpdate: now},
	}
	for _, reverse := range []bool{false, true} {
		ordered := append([]ReputationEntry(nil), entries...)
		if reverse {
			for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
				ordered[i], ordered[j] = ordered[j], ordered[i]
			}
		}
		path := filepath.Join(t.TempDir(), "reputation.mmdb")
		if err := NewDefaultWriter().CompileToMMDB(ordered, path); err != nil {
			t.Fatal(err)
		}
		r, err := NewReader(path, "", "")
		if err != nil {
			t.Fatal(err)
		}
		for ip, want := range map[string]int{"8.8.8.9": 75, "8.8.8.100": 60, "8.8.8.200": 60} {
			got, err := r.LookupAll(netip.MustParseAddr(ip))
			if err != nil || got.Score != want {
				t.Fatalf("reverse=%v %s score %#v err=%v want=%d", reverse, ip, got, err, want)
			}
		}
		r.Close()
	}
}

func TestIPv6TransitionRecordsDoNotAliasIPv4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transition.mmdb")
	entries := []ReputationEntry{
		{Prefix: netip.MustParsePrefix("168.144.132.25/32"), RiskScore: 80, Flags: EntryFlags{IsTor: true}},
		{Prefix: netip.MustParsePrefix("2002:a890:8419::a890:8419/128"), RiskScore: 32, Flags: EntryFlags{IsProxy: true}},
		{Prefix: netip.MustParsePrefix("2001:0000:4136:e378:8000:63bf:3fff:fdd2/128"), RiskScore: 45, Flags: EntryFlags{IsVPN: true}},
	}
	if err := NewDefaultWriter().CompileToMMDB(entries, path); err != nil {
		t.Fatal(err)
	}
	r, err := NewReader(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for ip, want := range map[string]int{"168.144.132.25": 80, "2002:a890:8419::a890:8419": 32, "2001:0000:4136:e378:8000:63bf:3fff:fdd2": 45} {
		got, err := r.LookupAll(netip.MustParseAddr(ip))
		if err != nil || got.Score != want {
			t.Fatalf("%s: got %#v, err %v, want %d", ip, got, err, want)
		}
	}
}
