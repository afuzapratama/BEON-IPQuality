package mmdb

import (
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

func snapshotFixture(t *testing.T, path string, score int, feeds []models.FeedFreshness) {
	t.Helper()
	writer := NewDefaultWriter()
	writer.SetFeedFreshness(feeds)
	if err := writer.CompileToMMDB([]ReputationEntry{{Prefix: netip.MustParsePrefix("8.8.8.0/24"), RiskScore: score, RiskLevel: "medium", ThreatType: "proxy", Sources: []string{"fixture"}, LastUpdate: time.Now()}}, path); err != nil {
		t.Fatal(err)
	}
}

func TestReloadChecksVersionAndPreservesPreviousSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reputation.mmdb")
	snapshotFixture(t, path, 40, nil)
	reader, err := NewReader(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	original := reader.Version()
	changed, err := reader.ReloadIfChanged(path, "", "")
	if err != nil || changed {
		t.Fatalf("unchanged snapshot: changed=%v err=%v", changed, err)
	}
	snapshotFixture(t, path, 80, nil)
	changed, err = reader.ReloadIfChanged(path, "", "")
	if err != nil || !changed || reader.Version() == original {
		t.Fatalf("new snapshot not loaded: %v %v", changed, err)
	}
	loaded := reader.Version()
	bad := filepath.Join(t.TempDir(), "bad.mmdb")
	if err := os.WriteFile(bad, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if changed, err = reader.ReloadIfChanged(bad, "", ""); err == nil || changed {
		t.Fatal("corrupt reload should fail")
	}
	result, err := reader.LookupAll(netip.MustParseAddr("8.8.8.8"))
	if err != nil || result.Score != 80 || result.DataVersion != loaded {
		t.Fatalf("lost previous snapshot: %+v %v", result, err)
	}
	reader.Close()
	if _, err := reader.ReloadIfChanged(path, "", ""); err == nil {
		t.Fatal("closed reader must not be resurrected")
	}
}

func TestFreshCompilationDoesNotHideStaleFeed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reputation.mmdb")
	old := time.Now().Add(-48 * time.Hour)
	snapshotFixture(t, path, 40, []models.FeedFreshness{{Feed: "daily", Source: "fixture", LastSuccess: &old, LatestStatus: "error", MaxAgeSeconds: (25 * time.Hour).Seconds()}})
	reader, err := NewReader(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	status := reader.Snapshot(time.Hour, time.Now())
	if !status.Stale || !status.FeedFreshnessKnown || len(status.Feeds) != 1 || !status.Feeds[0].Stale {
		t.Fatalf("fresh file hid stale feed: %+v", status)
	}
	if reader.Snapshot(time.Hour, time.Now().Add(72*time.Hour)).AgeSeconds < (71 * time.Hour).Seconds() {
		t.Fatal("artifact age missing")
	}
}

func TestConcurrentLookupAndReloadUseCompleteGenerations(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.mmdb"), filepath.Join(dir, "second.mmdb")
	snapshotFixture(t, first, 30, nil)
	snapshotFixture(t, second, 80, nil)
	reader, err := NewReader(first, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	v1 := reader.Version()
	candidate, err := NewReader(second, "", "")
	if err != nil {
		t.Fatal(err)
	}
	v2 := candidate.Version()
	candidate.Close()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				result, err := reader.LookupAll(netip.MustParseAddr("8.8.8.8"))
				if err != nil {
					t.Error(err)
					return
				}
				if (result.Score == 30 && result.DataVersion != v1) || (result.Score == 80 && result.DataVersion != v2) {
					t.Errorf("mixed generation: %+v", result)
				}
			}
		}()
	}
	for index := range 10 {
		path := first
		if index%2 == 0 {
			path = second
		}
		if _, err := reader.ReloadIfChanged(path, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

func TestReloadRejectsLossOfPreviouslyLoadedOptionalDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reputation.mmdb")
	optional := filepath.Join(t.TempDir(), "optional.mmdb")
	snapshotFixture(t, path, 40, nil)
	// A valid MMDB suffices here: this test exercises optional file availability,
	// independently of ASN lookup fields (covered by the pipeline fixture).
	snapshotFixture(t, optional, 30, nil)
	reader, err := NewReader(path, "", optional)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	old := reader.Version()
	if err := os.Remove(optional); err != nil {
		t.Fatal(err)
	}
	snapshotFixture(t, path, 80, nil)
	changed, err := reader.ReloadIfChanged(path, "", optional)
	if err == nil || changed || reader.Version() != old || !reader.HasASN() {
		t.Fatalf("lost optional database during reload: changed=%v err=%v", changed, err)
	}
	result, err := reader.LookupAll(netip.MustParseAddr("8.8.8.8"))
	if err != nil || result.Score != 40 {
		t.Fatalf("reputation advanced during rejected reload: %+v %v", result, err)
	}
}
