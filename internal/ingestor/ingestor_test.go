package ingestor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
)

func TestParseContentHandlesProxyURLsAndURLHosts(t *testing.T) {
	ing := &Ingestor{}
	feed := config.FeedConfig{Name: "Proxy", ThreatType: "proxy", Confidence: 0.8, Weight: 40}

	proxyEntries, err := ing.parseContent("socks5://8.8.8.8:1080\n1.1.1.1:8080\n", "ip_port", feed, "proxifly")
	if err != nil {
		t.Fatal(err)
	}
	if len(proxyEntries) != 2 || proxyEntries[0].IPString != "8.8.8.8" {
		t.Fatalf("unexpected proxy entries: %#v", proxyEntries)
	}
	if proxyEntries[0].Source != "Proxy/proxifly" {
		t.Fatalf("source = %q", proxyEntries[0].Source)
	}

	urlEntries, err := ing.parseContent("http://8.8.4.4/payload\nhttps://malware.example/file\n", "url", feed, "urlhaus")
	if err != nil {
		t.Fatal(err)
	}
	if len(urlEntries) != 1 || urlEntries[0].IPString != "8.8.4.4" {
		t.Fatalf("unexpected URL entries: %#v", urlEntries)
	}
}

func TestParseWhitelistEntriesIPv4AndIPv6(t *testing.T) {
	entries, err := parseWhitelistEntries("test", []string{"1.1.1.1", "2001:4860:4860::/48", "invalid", "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
}

func TestSkipInitialRequiresValidSnapshotBeforeStarting(t *testing.T) {
	cfg := &config.Config{}
	cfg.Ingestor.SkipInitial = true
	cfg.MMDB.ReputationPath = filepath.Join(t.TempDir(), "missing.mmdb")
	ing := &Ingestor{config: cfg}
	if err := ing.Start(context.Background()); err == nil {
		t.Fatal("expected missing snapshot error")
	}
	if ing.running {
		t.Fatal("scheduler must not start without valid snapshot")
	}
}
