package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"

	"github.com/lfrfrfr/beon-ipquality/internal/api/handlers"
	"github.com/lfrfrfr/beon-ipquality/internal/cache"
	"github.com/lfrfrfr/beon-ipquality/internal/compiler"
	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/database"
	"github.com/lfrfrfr/beon-ipquality/internal/ingestor"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

func TestPipelineWithPostgresRedisAndFixtureFeed(t *testing.T) {
	databaseURL, redisURL := os.Getenv("BEON_INTEGRATION_DATABASE_URL"), os.Getenv("BEON_INTEGRATION_REDIS_URL")
	if databaseURL == "" || redisURL == "" {
		t.Skip("set BEON_INTEGRATION_DATABASE_URL and BEON_INTEGRATION_REDIS_URL or run scripts/test-integration.sh")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("beon_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema+",public")
	parsed.RawQuery = query.Encode()
	db, err := database.NewPostgresDB(parsed.String(), 4, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_initial_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	var failing atomic.Bool
	var payload atomic.Value
	payload.Store("8.8.8.0/24\n2606:4700:4700::1111\n")
	feedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, payload.Load().(string))
	}))
	defer feedServer.Close()
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database.Postgres.URL = parsed.String()
	cfg.Database.Postgres.MaxConnections = 4
	cfg.Database.Postgres.MinConnections = 0
	cfg.MMDB.OutputPath = filepath.Join(t.TempDir(), "reputation.mmdb")
	cfg.MMDB.ReputationPath = cfg.MMDB.OutputPath
	cfg.MMDB.GeoLite2CityPath = ""
	cfg.MMDB.GeoLite2ASNPath = filepath.Join(t.TempDir(), "asn.mmdb")
	cfg.Ingestor.MaxRetries = 0
	cfg.Scoring.RiskThreshold = 20
	cfg.Scoring.Weights = map[string]int{"fixture": 20}
	cfg.Scoring.ASNBonuses = map[string]int{"datacenter": 10}
	asnTree, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "GeoLite2-ASN", Description: map[string]string{"en": "Fixture ASN"}, RecordSize: 28, IPVersion: 6})
	if err != nil {
		t.Fatal(err)
	}
	_, asnNetwork, _ := net.ParseCIDR("8.8.8.0/24")
	if err := asnTree.Insert(asnNetwork, mmdbtype.Map{"autonomous_system_number": mmdbtype.Uint32(15169), "autonomous_system_organization": mmdbtype.String("Fixture ASN")}); err != nil {
		t.Fatal(err)
	}
	asnFile, err := os.Create(cfg.MMDB.GeoLite2ASNPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := asnTree.WriteTo(asnFile); err != nil {
		t.Fatal(err)
	}
	asnFile.Close()
	cfg.MMDB.FeedsPath = filepath.Join(t.TempDir(), "feeds.yaml")
	feedYAML := fmt.Sprintf(`feeds:
  fixture:
    enabled: true
    name: Fixture Feed
    threat_type: proxy
    confidence: 0.8
    weight: 50
    schedule: '@hourly'
    sources:
      - name: fixture_source
        url: %q
        format: plain
formats:
  plain:
    description: fixture
whitelist:
  enabled: true
  sources:
    - name: fixture_allow
      ranges: [8.8.8.9]
`, feedServer.URL)
	if err := os.WriteFile(cfg.MMDB.FeedsPath, []byte(feedYAML), 0600); err != nil {
		t.Fatal(err)
	}
	feeds, err := config.LoadFeeds(cfg.MMDB.FeedsPath)
	if err != nil {
		t.Fatal(err)
	}
	ing, err := ingestor.New(cfg, feeds, db)
	if err != nil {
		t.Fatal(err)
	}
	if _, entries, stored, err := ing.RunOnce(ctx); err != nil || entries != 2 || stored != 2 {
		t.Fatalf("ingest: entries=%d stored=%d err=%v", entries, stored, err)
	}
	comp, err := compiler.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer comp.Close()
	// A different compiler process holding the database lock must prevent
	// this one from loading a second large aggregation/writer into memory.
	func() {
		holder, err := db.Pool().Acquire(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer holder.Release()
		if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock(741852964)"); err != nil {
			t.Fatal(err)
		}
		defer holder.Exec(context.Background(), "SELECT pg_advisory_unlock(741852964)")
		waitCtx, waitCancel := context.WithTimeout(ctx, 150*time.Millisecond)
		defer waitCancel()
		if err := comp.Compile(waitCtx); err == nil {
			t.Fatal("compiler bypassed cross-process memory lock")
		}
	}()
	if err := comp.Compile(ctx); err != nil {
		t.Fatal(err)
	}
	// Disabled source must not leak unexpired DB rows into a new snapshot.
	originalSnapshot, err := os.ReadFile(cfg.MMDB.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.MMDB.FeedsPath, []byte(strings.Replace(feedYAML, "enabled: true", "enabled: false", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := comp.Compile(ctx); err == nil {
		t.Fatal("disabled-only source unexpectedly compiled a snapshot")
	}
	preserved, err := os.ReadFile(cfg.MMDB.OutputPath)
	if err != nil || !bytes.Equal(preserved, originalSnapshot) {
		t.Fatal("disabled-only compilation damaged the valid snapshot")
	}
	if err := os.WriteFile(cfg.MMDB.FeedsPath, []byte(feedYAML), 0600); err != nil {
		t.Fatal(err)
	}
	reader, err := mmdb.NewReader(cfg.MMDB.OutputPath, "", cfg.MMDB.GeoLite2ASNPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	redisCache, err := cache.NewRedisCache(cache.Config{URL: redisURL, Prefix: schema + ":", OperationTimeout: 50 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer redisCache.Close()
	defer redisCache.Clear(context.Background())
	handlers.SetMMDBReader(reader)
	handlers.SetCache(redisCache)
	handlers.SetMMDBConfig(handlers.MMDBConfig{MaxAge: time.Hour})
	defer func() {
		handlers.SetCache(nil)
		handlers.SetMMDBReader(nil)
		handlers.SetMMDBConfig(handlers.MMDBConfig{})
	}()
	app := fiber.New()
	defer app.Shutdown()
	app.Get("/health", handlers.HealthCheck("integration"))
	app.Get("/check/:ip", handlers.CheckIP())
	check := func(ip string) models.IPCheckResult {
		t.Helper()
		resp, err := app.Test(httptest.NewRequest("GET", "/check/"+ip, nil))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("lookup %s status %d", ip, resp.StatusCode)
		}
		var result models.IPCheckResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := check("8.8.8.8")
	if first.Score != 24 || !first.Risky || first.Cached || first.ASN == nil || first.ASN.ASNType != "datacenter" || !first.IsDatacenter {
		t.Fatalf("configuration/ASN not applied: %+v", first)
	}
	if second := check("8.8.8.8"); !second.Cached || second.DataVersion != first.DataVersion {
		t.Fatalf("Redis cache miss: %+v", second)
	}
	if allowed := check("8.8.8.9"); !allowed.Whitelisted || allowed.Score != 0 {
		t.Fatalf("whitelist: %+v", allowed)
	}
	if ipv6 := check("2606:4700:4700::1111"); ipv6.Score != 16 {
		t.Fatalf("IPv6 score: %+v", ipv6)
	}

	// A smaller complete source expires removed IPs instead of retaining signals.
	payload.Store("2606:4700:4700::1111\n")
	if _, _, stored, err := ing.RunOnce(ctx); err != nil || stored != 1 {
		t.Fatalf("smaller source: %d %v", stored, err)
	}
	var active int
	if err := db.Pool().QueryRow(ctx, "SELECT count(*) FROM ip_reputation WHERE expires_at>NOW()").Scan(&active); err != nil || active != 1 {
		t.Fatalf("removed IP remained active: %d %v", active, err)
	}
	if err := comp.Compile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReloadIfChanged(cfg.MMDB.OutputPath, "", cfg.MMDB.GeoLite2ASNPath); err != nil {
		t.Fatal(err)
	}
	if removed := check("8.8.8.8"); removed.Score != 0 || removed.IsProxy {
		t.Fatalf("removed source signal leaked: %+v", removed)
	}
	payload.Store("8.8.8.0/24\n2606:4700:4700::1111\n")
	if _, _, _, err := ing.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := comp.Compile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReloadIfChanged(cfg.MMDB.OutputPath, "", cfg.MMDB.GeoLite2ASNPath); err != nil {
		t.Fatal(err)
	}
	// Recompiling after history has aged must not disguise an old feed as fresh.
	if _, err := db.Pool().Exec(ctx, "UPDATE feed_fetch_history SET created_at=NOW()-INTERVAL '3 hours'"); err != nil {
		t.Fatal(err)
	}
	if err := comp.Compile(ctx); err != nil {
		t.Fatal(err)
	}
	if changed, err := reader.ReloadIfChanged(cfg.MMDB.OutputPath, "", cfg.MMDB.GeoLite2ASNPath); err != nil || !changed {
		t.Fatalf("stale snapshot reload: %v %v", changed, err)
	}
	for _, path := range []string{"/health", "/check/8.8.8.8"} {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 503 {
			t.Fatalf("stale data at %s returned %d", path, resp.StatusCode)
		}
	}
	// A failed refresh is reported, recorded, and does not reset last success.
	failing.Store(true)
	if _, _, _, err := ing.RunOnce(ctx); err == nil {
		t.Fatal("failed feed returned success")
	}
	var status string
	if err := db.Pool().QueryRow(ctx, "SELECT status FROM feed_fetch_history ORDER BY id DESC LIMIT 1").Scan(&status); err != nil || status != "error" {
		t.Fatalf("failed feed history: %s %v", status, err)
	}
	failing.Store(false)
	// Batch storage failures must propagate instead of incrementing success history.
	if _, err := db.Pool().Exec(ctx, `ALTER TABLE ip_reputation ADD CONSTRAINT reject_fixture CHECK (source <> 'Fixture Feed/fixture_source') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ing.RunOnce(ctx); err == nil {
		t.Fatal("failed storage returned success")
	}
	if err := db.Pool().QueryRow(ctx, "SELECT count(*) FROM ip_reputation WHERE expires_at>NOW()").Scan(&active); err != nil || active != 2 {
		t.Fatal("failed publication altered previous complete source")
	}
	if _, err := db.Pool().Exec(ctx, "ALTER TABLE ip_reputation DROP CONSTRAINT reject_fixture"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ing.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err := comp.Compile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReloadIfChanged(cfg.MMDB.OutputPath, "", cfg.MMDB.GeoLite2ASNPath); err != nil {
		t.Fatal(err)
	}
	current := check("8.8.8.8")
	if current.Cached || current.DataVersion == first.DataVersion {
		t.Fatal("old Redis generation reused")
	}
	// Disconnected Redis must leave the MMDB lookup usable.
	redisCache.Close()
	if fallback := check("8.8.8.10"); fallback.Cached || fallback.Score != 24 {
		t.Fatalf("Redis outage did not fall back: %+v", fallback)
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO feed_fetch_history(feed_name,source_url,status,created_at) VALUES ('retention-fixture','fixture','success',NOW()-INTERVAL '100 days'),('retention-fixture','fixture','success',NOW()-INTERVAL '50 days'),('retention-fixture','fixture','error',NOW()-INTERVAL '45 days')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO ip_reputation(ip_start,ip_end,source,threat_type,expires_at) VALUES ('9.9.9.9','9.9.9.9','retention-fixture','proxy',NOW()-INTERVAL '40 days')`); err != nil {
		t.Fatal(err)
	}
	if err := db.PruneHistory(ctx, 30*24*time.Hour, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	var expiredRows int
	if err := db.Pool().QueryRow(ctx, "SELECT count(*) FROM ip_reputation WHERE source='retention-fixture'").Scan(&expiredRows); err != nil || expiredRows != 0 {
		t.Fatal("expired reputation history was not pruned")
	}
	var retained int
	if err := db.Pool().QueryRow(ctx, "SELECT count(*) FROM feed_fetch_history WHERE feed_name='retention-fixture'").Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("retention lost latest success/attempt: %d %v", retained, err)
	}
	bytesBefore, err := os.ReadFile(cfg.MMDB.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool().Exec(ctx, "DELETE FROM ip_reputation; DELETE FROM whitelist"); err != nil {
		t.Fatal(err)
	}
	if err := comp.Compile(ctx); err == nil {
		t.Fatal("empty compilation succeeded")
	}
	bytesAfter, err := os.ReadFile(cfg.MMDB.OutputPath)
	if err != nil || string(bytesAfter) != string(bytesBefore) {
		t.Fatal("empty compilation damaged previous snapshot")
	}
	// Complete-range ASN lookup must not label a wider range with its first IP's ASN.
	info, err := reader.LookupASNPrefix(netip.MustParsePrefix("8.8.0.0/16"))
	if err != nil || info != nil {
		t.Fatalf("cross-ASN prefix incorrectly classified: %+v %v", info, err)
	}
}
