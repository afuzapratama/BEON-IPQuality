package handlers

import (
	"context"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/lfrfrfr/beon-ipquality/internal/cache"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

type fixtureCache struct {
	cache.NoOpCache
	results map[string]models.IPCheckResult
	slow    bool
}

func (c *fixtureCache) Get(ctx context.Context, key string) (*models.IPCheckResult, error) {
	if c.slow {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if result, ok := c.results[key]; ok {
		return &result, nil
	}
	return nil, nil
}
func (c *fixtureCache) Set(ctx context.Context, key string, result *models.IPCheckResult) error {
	if c.slow {
		<-ctx.Done()
		return ctx.Err()
	}
	c.results[key] = *result
	return nil
}

func TestCacheFollowsSnapshotVersionAndFallsBackOnTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reputation.mmdb")
	write := func(score int) {
		t.Helper()
		if err := mmdb.NewDefaultWriter().CompileToMMDB([]mmdb.ReputationEntry{{Prefix: netip.MustParsePrefix("8.8.8.8/32"), RiskScore: score, RiskLevel: "medium", ThreatType: "proxy"}}, path); err != nil {
			t.Fatal(err)
		}
	}
	write(40)
	reader, err := mmdb.NewReader(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	SetMMDBReader(reader)
	SetMMDBConfig(MMDBConfig{MaxAge: time.Hour})
	SetCacheTimeout(10 * time.Millisecond)
	t.Cleanup(func() {
		SetCache(nil)
		SetMMDBReader(nil)
		SetMMDBConfig(MMDBConfig{})
		SetCacheTimeout(50 * time.Millisecond)
		reader.Close()
	})
	memory := &fixtureCache{results: make(map[string]models.IPCheckResult)}
	SetCache(memory)
	ip := netip.MustParseAddr("8.8.8.8")
	first, err := performIPCheck(ip, time.Now())
	if err != nil || first.Cached {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := performIPCheck(ip, time.Now())
	if err != nil || !second.Cached {
		t.Fatalf("second: %+v %v", second, err)
	}
	write(80)
	if _, err := reader.ReloadIfChanged(path, "", ""); err != nil {
		t.Fatal(err)
	}
	newer, err := performIPCheck(ip, time.Now())
	if err != nil || newer.Cached || newer.Score != 80 || newer.DataVersion == first.DataVersion {
		t.Fatalf("old cache leaked after reload: %+v %v", newer, err)
	}
	memory.slow = true
	start := time.Now()
	fallback, err := performIPCheck(ip, start)
	if err != nil || fallback.Cached || fallback.Score != 80 || time.Since(start) > time.Second {
		t.Fatalf("slow cache blocked lookup: %+v %v", fallback, err)
	}
	SetMMDBReader(nil)
	memory.slow = false
	if _, err := performIPCheck(ip, time.Now()); err == nil {
		t.Fatal("cache must not mask unavailable reader")
	}
}
