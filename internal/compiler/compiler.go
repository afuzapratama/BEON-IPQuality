package compiler

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	internalmetrics "github.com/lfrfrfr/beon-ipquality/internal/metrics"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/internal/scoring"
	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// Compiler compiles IP reputation data into MMDB format
type Compiler struct {
	config      *config.Config
	db          *pgxpool.Pool
	mmdbWriter  *mmdb.Writer
	scorer      *scoring.Scorer
	mu          sync.Mutex
	lastCompile time.Time
	asnReader   *mmdb.Reader
	asnTypes    map[int]models.ASNInfo
}

// New creates a new Compiler instance
func New(cfg *config.Config) (*Compiler, error) {
	// Connect to PostgreSQL
	poolConfig, err := pgxpool.ParseConfig(cfg.Database.Postgres.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.Database.Postgres.MaxConnections)
	poolConfig.MinConns = int32(cfg.Database.Postgres.MinConnections)

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create MMDB writer
	writerConfig := reputationWriterConfig(cfg.MMDB.RecordSize)
	mmdbWriter := mmdb.NewWriter(writerConfig)

	feeds, err := config.LoadFeeds(cfg.MMDB.FeedsPath)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("load compiler feed configuration: %w", err)
	}
	scorer := scoring.FromConfig(cfg.Scoring, feeds)

	return &Compiler{
		config:     cfg,
		db:         pool,
		mmdbWriter: mmdbWriter,
		scorer:     scorer,
	}, nil
}

// reputationWriterConfig includes private/bogon networks carried by feeds and
// local whitelists. Rejecting reserved networks aborts otherwise valid snapshots.
func reputationWriterConfig(recordSize int) mmdb.WriterConfig {
	return mmdb.WriterConfig{
		DatabaseType:        "BEON-IPReputation",
		Description:         "BEON IP Reputation Database",
		RecordSize:          recordSize,
		IPVersion:           0,
		IncludeReservedNets: true,
	}
}

// Close closes database connections
func (c *Compiler) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.asnReader != nil {
		c.asnReader.Close()
	}
	if c.db != nil {
		c.db.Close()
	}
}

// Compile compiles the reputation database to MMDB
func (c *Compiler) Compile(ctx context.Context) (compileErr error) {
	start := time.Now()
	defer func() {
		status := "success"
		if compileErr != nil {
			status = "error"
		}
		internalmetrics.Compilations.WithLabelValues(status).Inc()
		internalmetrics.CompileDuration.Observe(time.Since(start).Seconds())
	}()
	c.mu.Lock()
	defer c.mu.Unlock()

	logger.Info("Starting MMDB compilation...")
	startTime := time.Now()

	// Refresh optional ASN metadata from the curated PostgreSQL classification.
	if err := c.loadASNTypes(ctx); err != nil {
		return err
	}
	candidate, err := mmdb.NewReader("", "", c.config.MMDB.GeoLite2ASNPath)
	if err != nil {
		return err
	}
	if c.asnReader != nil && c.asnReader.HasASN() && c.config.MMDB.GeoLite2ASNPath != "" && !candidate.HasASN() {
		candidate.Close()
		return fmt.Errorf("refusing compilation after loss of loaded ASN database")
	}
	if c.asnReader != nil {
		c.asnReader.Close()
	}
	c.asnReader = candidate

	// Fetch reputation data from database
	reputations, err := c.fetchReputationData(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch reputation data: %w", err)
	}

	logger.Info(fmt.Sprintf("Fetched %d reputation entries from database", len(reputations)))
	if len(reputations) == 0 {
		return fmt.Errorf("no active reputation rows from enabled sources; retaining prior snapshot")
	}

	entries, err := c.aggregateReputations(reputations, time.Now())
	if err != nil {
		return fmt.Errorf("failed to aggregate reputation data: %w", err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("no usable reputation networks; refusing whitelist-only snapshot")
	}
	whitelists, err := c.fetchWhitelistData(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch whitelist data: %w", err)
	}
	for _, prefix := range whitelists {
		entries = append(entries, mmdb.ReputationEntry{
			Prefix:      prefix,
			RiskLevel:   "clean",
			Whitelisted: true,
			LastUpdate:  time.Now(),
		})
	}
	feeds, err := c.fetchFeedFreshness(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("load feed freshness: %w", err)
	}
	c.mmdbWriter.SetFeedFreshness(feeds)
	if err := c.publishEntries(entries); err != nil {
		return err
	}

	c.lastCompile = time.Now()
	internalmetrics.CompileLastSuccess.Set(float64(c.lastCompile.Unix()))
	internalmetrics.MMDBEntries.Set(float64(len(entries)))
	logger.Info(fmt.Sprintf("MMDB compilation complete in %v, output: %s", time.Since(startTime), c.config.MMDB.OutputPath))

	return nil
}

// fetchReputationData fetches all active reputation data from the database
func (c *Compiler) fetchReputationData(ctx context.Context) ([]models.IPReputation, error) {
	feeds, err := config.LoadFeeds(c.config.MMDB.FeedsPath)
	if err != nil {
		return nil, fmt.Errorf("load active sources: %w", err)
	}
	enabled := enabledSourceIDs(feeds)

	query := `
		SELECT 
			id,
			CASE
				WHEN cidr IS NOT NULL THEN cidr::text
				WHEN family(ip_start) = 4 THEN host(ip_start) || '/32'
				ELSE host(ip_start) || '/128'
			END as ip_range,
			source,
			threat_type,
			confidence,
			weight,
			first_seen,
			last_seen
		FROM ip_reputation
		WHERE (expires_at IS NULL OR expires_at > NOW())
 AND source = ANY($1::text[])
		ORDER BY last_seen DESC
	`

	rows, err := c.db.Query(ctx, query, enabled)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var reputations []models.IPReputation

	for rows.Next() {
		var rep models.IPReputation
		err := rows.Scan(
			&rep.ID,
			&rep.IPRange,
			&rep.Source,
			&rep.ThreatType,
			&rep.Confidence,
			&rep.Weight,
			&rep.FirstSeen,
			&rep.LastSeen,
		)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to scan row: %v", err))
			continue
		}
		reputations = append(reputations, rep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return reputations, nil
}

type reputationAggregate struct {
	prefix       netip.Prefix
	threats      []models.Threat
	sources      map[string]struct{}
	threatTypes  map[string]struct{}
	flags        mmdb.EntryFlags
	primaryType  string
	primaryValue float64
	confidence   float64
	lastUpdate   time.Time
}

func (c *Compiler) aggregateReputations(reputations []models.IPReputation, now time.Time) ([]mmdb.ReputationEntry, error) {
	aggregates := make(map[string]*reputationAggregate)
	for _, reputation := range reputations {
		prefix, err := parseReputationPrefix(reputation.IPRange)
		if err != nil {
			logger.Warn(fmt.Sprintf("Skipping invalid reputation range %q: %v", reputation.IPRange, err))
			continue
		}
		prefix = prefix.Masked()
		key := prefix.String()
		aggregate := aggregates[key]
		if aggregate == nil {
			aggregate = &reputationAggregate{
				prefix:      prefix,
				sources:     make(map[string]struct{}),
				threatTypes: make(map[string]struct{}),
			}
			aggregates[key] = aggregate
		}

		aggregate.threats = append(aggregate.threats, models.Threat{
			ThreatType: reputation.ThreatType,
			Source:     reputation.Source,
			Confidence: reputation.Confidence,
			Weight:     reputation.Weight,
			LastSeen:   reputation.LastSeen,
		})
		aggregate.sources[reputation.Source] = struct{}{}
		aggregate.threatTypes[reputation.ThreatType] = struct{}{}
		aggregate.flags = mergeFlags(aggregate.flags, flagsForThreatType(reputation.ThreatType))
		primaryValue := float64(reputation.Weight) * reputation.Confidence
		if primaryValue > aggregate.primaryValue {
			aggregate.primaryValue = primaryValue
			aggregate.primaryType = reputation.ThreatType
		}
		if reputation.Confidence > aggregate.confidence {
			aggregate.confidence = reputation.Confidence
		}
		if reputation.LastSeen.After(aggregate.lastUpdate) {
			aggregate.lastUpdate = reputation.LastSeen
		}
	}

	keys := make([]string, 0, len(aggregates))
	for key := range aggregates {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entries := make([]mmdb.ReputationEntry, 0, len(keys))
	for _, key := range keys {
		aggregate := aggregates[key]
		sources := mapKeys(aggregate.sources)
		threatTypes := mapKeys(aggregate.threatTypes)
		asn, err := c.classifyASN(aggregate.prefix)
		if err != nil {
			return nil, err
		}
		if asn != nil && (asn.ASNType == "datacenter" || asn.ASNType == "hosting") {
			aggregate.flags.IsDatacenter = true
		}
		score := c.scorer.CalculateScore(aggregate.threats, asn, now)
		entries = append(entries, mmdb.ReputationEntry{
			Prefix:        aggregate.prefix,
			RiskScore:     score,
			Evidence:      c.scorer.Evidence(aggregate.threats, asn, now),
			ScoreCap:      c.scorer.MaxScore(),
			RiskThreshold: c.riskThreshold(),
			RiskLevel:     c.scorer.ClassifyRisk(score),
			ThreatType:    aggregate.primaryType,
			ThreatTypes:   threatTypes,
			Confidence:    aggregate.confidence,
			Sources:       sources,
			Flags:         aggregate.flags,
			LastUpdate:    aggregate.lastUpdate,
			ASN:           asn,
		})
	}
	return entries, nil
}

func (c *Compiler) fetchWhitelistData(ctx context.Context) ([]netip.Prefix, error) {
	rows, err := c.db.Query(ctx, `
		SELECT CASE
			WHEN cidr IS NOT NULL THEN cidr::text
			WHEN family(ip_start) = 4 THEN host(ip_start) || '/32'
			ELSE host(ip_start) || '/128'
		END
		FROM whitelist
		WHERE permanent = true OR expires_at IS NULL OR expires_at > NOW()
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prefixes []netip.Prefix
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("invalid whitelist prefix %q: %w", value, err)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, rows.Err()
}

func parseReputationPrefix(value string) (netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix, nil
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	if addr.Is4() {
		return netip.PrefixFrom(addr, 32), nil
	}
	return netip.PrefixFrom(addr, 128), nil
}

func mapKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func flagsForThreatType(threatType string) mmdb.EntryFlags {
	var flags mmdb.EntryFlags
	switch threatType {
	case "tor":
		flags.IsTor = true
	case "vpn", "anonymous":
		flags.IsVPN = true
	case "proxy":
		flags.IsProxy = true
	case "datacenter", "hosting":
		flags.IsDatacenter = true
	case "botnet_c2", "botnet":
		flags.IsBotnet = true
	case "malware", "malicious":
		flags.IsMalware = true
	case "spam":
		flags.IsSpam = true
	case "attack", "hijacked", "suspicious", "bruteforce":
		flags.IsAttacker = true
	}
	return flags
}

func mergeFlags(left, right mmdb.EntryFlags) mmdb.EntryFlags {
	return mmdb.EntryFlags{
		IsTor:        left.IsTor || right.IsTor,
		IsVPN:        left.IsVPN || right.IsVPN,
		IsProxy:      left.IsProxy || right.IsProxy,
		IsDatacenter: left.IsDatacenter || right.IsDatacenter,
		IsBotnet:     left.IsBotnet || right.IsBotnet,
		IsMalware:    left.IsMalware || right.IsMalware,
		IsSpam:       left.IsSpam || right.IsSpam,
		IsAttacker:   left.IsAttacker || right.IsAttacker,
	}
}

// GetLastCompileTime returns the last compilation time
func (c *Compiler) GetLastCompileTime() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastCompile
}

// Stats returns compilation statistics
type Stats struct {
	LastCompile  time.Time     `json:"last_compile"`
	TotalEntries int           `json:"total_entries"`
	CompileCount int           `json:"compile_count"`
	LastDuration time.Duration `json:"last_duration"`
}

func (c *Compiler) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{
		LastCompile: c.lastCompile,
	}
}

func (c *Compiler) publishEntries(entries []mmdb.ReputationEntry) error {
	if len(entries) == 0 {
		return fmt.Errorf("no reputation or whitelist data to compile; refusing to publish an empty snapshot")
	}
	if err := c.mmdbWriter.CompileToMMDB(entries, c.config.MMDB.OutputPath); err != nil {
		return fmt.Errorf("failed to compile MMDB: %w", err)
	}
	return nil
}

func (c *Compiler) loadASNTypes(ctx context.Context) error {
	rows, err := c.db.Query(ctx, "SELECT asn, COALESCE(org,''), COALESCE(asn_type,'') FROM asn_info")
	if err != nil {
		return fmt.Errorf("load ASN classifications: %w", err)
	}
	defer rows.Close()
	types := make(map[int]models.ASNInfo)
	for rows.Next() {
		var info models.ASNInfo
		if err := rows.Scan(&info.ASN, &info.Org, &info.ASNType); err != nil {
			return err
		}
		types[info.ASN] = info
	}
	if err := rows.Err(); err != nil {
		return err
	}
	c.asnTypes = types
	return nil
}

func (c *Compiler) classifyASN(prefix netip.Prefix) (*models.ASNInfo, error) {
	if c.asnReader == nil {
		return nil, nil
	}
	info, err := c.asnReader.LookupASNPrefix(prefix)
	if err != nil {
		return nil, fmt.Errorf("ASN lookup for %s: %w", prefix, err)
	}
	if info == nil {
		return nil, nil
	}
	if known, ok := c.asnTypes[info.ASN]; ok {
		info.ASNType = known.ASNType
		info.Type = known.ASNType
	}
	return info, nil
}

func (c *Compiler) riskThreshold() int {
	if c.config != nil && c.config.Scoring.RiskThreshold > 0 {
		return c.config.Scoring.RiskThreshold
	}
	return 50
}

// enabledSourceIDs excludes removed/disabled sources even while their DB TTL is
// still live, and accepts canonical and historic human-readable source IDs.
func enabledSourceIDs(feeds *config.FeedsConfig) []string {
	set := make(map[string]struct{})
	for id, feed := range feeds.Feeds {
		if !feed.Enabled {
			continue
		}
		for _, source := range feed.Sources {
			suffix := ""
			if source.Name != "" {
				suffix = "/" + source.Name
			}
			set[id+suffix] = struct{}{}
			set[feed.Name+suffix] = struct{}{}
		}
	}
	return mapKeys(set)
}
