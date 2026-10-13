package ingestor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/database"
	internalmetrics "github.com/lfrfrfr/beon-ipquality/internal/metrics"
	"github.com/lfrfrfr/beon-ipquality/internal/mmdb"
	"github.com/lfrfrfr/beon-ipquality/pkg/iputil"
	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// Ingestor handles fetching and processing threat feeds
type Ingestor struct {
	config      *config.Config
	feedsConfig *config.FeedsConfig
	httpClient  *http.Client
	db          *database.PostgresDB
	cron        *cron.Cron
	mu          sync.RWMutex
	running     bool
	wg          sync.WaitGroup
}

// New creates a new Ingestor instance
func New(cfg *config.Config, feedsCfg *config.FeedsConfig, db *database.PostgresDB) (*Ingestor, error) {
	httpClient := &http.Client{
		Timeout: cfg.Ingestor.HTTPTimeout,
	}

	return &Ingestor{
		config:      cfg,
		feedsConfig: feedsCfg,
		httpClient:  httpClient,
		db:          db,
		cron:        cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger))), // Standard 5-field cron format (minute, hour, day, month, weekday)
	}, nil
}

// Start starts the ingestor service
func (i *Ingestor) Start(ctx context.Context) error {
	if i.config.Ingestor.SkipInitial {
		reader, err := mmdb.NewReader(i.config.MMDB.ReputationPath, "", "")
		if err != nil {
			return fmt.Errorf("cannot skip initial ingestion without valid snapshot: %w", err)
		}
		reader.Close()
	}
	i.mu.Lock()
	if i.running {
		i.mu.Unlock()
		return fmt.Errorf("ingestor already running")
	}
	i.running = true
	i.mu.Unlock()

	// Schedule feeds
	enabledFeeds := i.feedsConfig.GetIngestibleFeeds()
	for name, feed := range enabledFeeds {
		feedName := name
		feedConfig := feed

		logger.Info(fmt.Sprintf("Scheduling feed: %s with schedule: %s", feedName, feedConfig.Schedule))

		_, err := i.cron.AddFunc(feedConfig.Schedule, func() {
			i.processFeed(ctx, feedName, feedConfig)
		})
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to schedule feed %s: %v", feedName, err))
		}
	}

	// Start cron scheduler
	if i.feedsConfig.Whitelist.Enabled {
		if _, err := i.cron.AddFunc("@daily", func() {
			if _, syncErr := i.processWhitelists(ctx); syncErr != nil {
				logger.Error(fmt.Sprintf("Failed to refresh whitelists: %v", syncErr))
			}
		}); err != nil {
			logger.Error(fmt.Sprintf("Failed to schedule whitelist refresh: %v", err))
		}
	}
	if _, err := i.cron.AddFunc("@daily", func() {
		if err := i.db.PruneHistory(ctx, i.config.Ingestor.ReputationRetention, i.config.Ingestor.HistoryRetention); err != nil {
			logger.Error(fmt.Sprintf("Retention cleanup failed: %v", err))
		}
	}); err != nil {
		return err
	}
	i.cron.Start()

	if !i.config.Ingestor.SkipInitial {
		// Run initial fetch for all feeds
		logger.Info("Running initial fetch for all feeds...")
		i.runAllFeeds(ctx)
		if _, err := i.processWhitelists(ctx); err != nil {
			logger.Error(fmt.Sprintf("Initial whitelist sync failed: %v", err))
		}
	}

	// Wait for context cancellation
	<-ctx.Done()

	return nil
}

// Stop stops the ingestor service
func (i *Ingestor) Stop() {
	i.mu.Lock()
	defer i.mu.Unlock()

	if !i.running {
		return
	}

	stopped := i.cron.Stop()
	<-stopped.Done()
	i.wg.Wait()
	i.running = false
}

// RunOnce runs all feeds once and returns statistics (for --once mode)
func (i *Ingestor) RunOnce(ctx context.Context) (totalFeeds, totalEntries, totalStored int, err error) {
	enabledFeeds := i.feedsConfig.GetIngestibleFeeds()
	totalFeeds = len(enabledFeeds)

	// Results tracking with mutex for thread safety
	var mu sync.Mutex
	var runErrors []error

	// Use semaphore for concurrency control
	sem := make(chan struct{}, i.config.Ingestor.Concurrency)

	for name, feed := range enabledFeeds {
		sem <- struct{}{} // Acquire

		i.wg.Add(1)
		go func(feedName string, feedConfig config.FeedConfig) {
			defer i.wg.Done()
			defer func() { <-sem }() // Release

			entries, stored, feedErr := i.processFeedWithStats(ctx, feedName, feedConfig)

			mu.Lock()
			totalEntries += entries
			totalStored += stored
			if feedErr != nil {
				runErrors = append(runErrors, feedErr)
			}
			mu.Unlock()
		}(name, feed)
	}

	// Wait for all feeds to complete
	i.wg.Wait()

	if _, whitelistErr := i.processWhitelists(ctx); whitelistErr != nil {
		runErrors = append(runErrors, whitelistErr)
	}
	err = errors.Join(runErrors...)

	return
}

// runAllFeeds runs all enabled feeds
func (i *Ingestor) runAllFeeds(ctx context.Context) {
	enabledFeeds := i.feedsConfig.GetIngestibleFeeds()

	// Use semaphore for concurrency control
	sem := make(chan struct{}, i.config.Ingestor.Concurrency)

	for name, feed := range enabledFeeds {
		sem <- struct{}{} // Acquire

		i.wg.Add(1)
		go func(feedName string, feedConfig config.FeedConfig) {
			defer i.wg.Done()
			defer func() { <-sem }() // Release

			i.processFeed(ctx, feedName, feedConfig)
		}(name, feed)
	}

	// Wait for all feeds to complete
	i.wg.Wait()
}

// processFeed runs a scheduled feed through the same storage/history path as bootstrap.
func (i *Ingestor) processFeed(ctx context.Context, feedName string, feedConfig config.FeedConfig) {
	_, _, err := i.processFeedWithStats(ctx, feedName, feedConfig)
	if err != nil {
		logger.Error(fmt.Sprintf("Feed %s failed: %v", feedName, err))
	}
}

func (i *Ingestor) processFeedWithStats(ctx context.Context, feedName string, feedConfig config.FeedConfig) (totalEntries, totalStored int, err error) {
	var failures []error
	for _, source := range feedConfig.Sources {
		if err := ctx.Err(); err != nil {
			return totalEntries, totalStored, err
		}
		start := time.Now()
		entries, sourceErr := i.fetchSource(ctx, source, feedConfig)
		stored := 0
		if sourceErr == nil {
			budget, budgetErr := config.FreshnessBudget(feedConfig.Schedule, start, i.config.MMDB.CompileInterval, i.config.MMDB.ReloadInterval, i.config.Ingestor.HTTPTimeout*time.Duration(i.config.Ingestor.MaxRetries+1)+i.config.Ingestor.RetryDelay*time.Duration(i.config.Ingestor.MaxRetries))
			if budgetErr != nil {
				sourceErr = budgetErr
			} else {
				stored, sourceErr = i.storeSourceSnapshot(ctx, entries, time.Now().Add(2*budget))
			}
		}
		totalEntries += len(entries)
		totalStored += stored
		status, message := "success", ""
		if sourceErr != nil {
			status = "error"
			message = sourceErr.Error()
			failures = append(failures, fmt.Errorf("%s/%s: %w", feedName, source.Name, sourceErr))
		}
		internalmetrics.FeedFetches.WithLabelValues(feedName, source.Name, status).Inc()
		internalmetrics.FeedDuration.WithLabelValues(feedName, source.Name).Observe(time.Since(start).Seconds())
		if sourceErr == nil {
			internalmetrics.FeedLastSuccess.WithLabelValues(feedName, source.Name).SetToCurrentTime()
		}
		// Storage/history errors must not silently produce a successful bootstrap.
		historyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		historyErr := i.db.RecordFeedFetch(historyCtx, feedName, source.URL, status, len(entries), stored, time.Since(start), message)
		cancel()
		if historyErr != nil {
			failures = append(failures, fmt.Errorf("record feed history: %w", historyErr))
		}
		logger.Info(fmt.Sprintf("Feed %s/%s: %s, fetched %d, stored %d", feedName, source.Name, status, len(entries), stored))
	}
	return totalEntries, totalStored, errors.Join(failures...)
}

// fetchSource fetches and parses a single source
func (i *Ingestor) fetchSource(ctx context.Context, source config.SourceConfig, feedConfig config.FeedConfig) ([]models.FeedEntry, error) {
	// Retry logic: create a fresh request for each attempt so that the
	// request object is never reused after a response has been read.
	var resp *http.Response
	var err error
	var lastStatus int
	for attempt := 0; attempt <= i.config.Ingestor.MaxRetries; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", source.URL, nil)
		if reqErr != nil {
			return nil, fmt.Errorf("failed to create request: %w", reqErr)
		}
		req.Header.Set("User-Agent", i.config.Ingestor.UserAgent)

		resp, err = i.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if resp != nil {
			lastStatus = resp.StatusCode
			resp.Body.Close()
			resp = nil
		}

		if attempt < i.config.Ingestor.MaxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(i.config.Ingestor.RetryDelay):
			}
		}
	}

	if resp == nil && lastStatus != 0 {
		return nil, fmt.Errorf("unexpected status code after retries: %d", lastStatus)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch after %d retries: %w", i.config.Ingestor.MaxRetries, err)
	}
	defer resp.Body.Close()

	// Limit body read to 100 MB and reject truncated feeds.
	const maxBodyBytes = 100 * 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(body) > maxBodyBytes {
		return nil, fmt.Errorf("feed exceeds maximum size of %d bytes", maxBodyBytes)
	}

	// Parse based on format
	entries, err := i.parseContent(string(body), source.Format, feedConfig, source.Name)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("feed returned no valid IP entries")
	}
	return entries, nil
}

// parseContent parses the content based on format
func (i *Ingestor) parseContent(content, format string, feedConfig config.FeedConfig, sourceName string) ([]models.FeedEntry, error) {
	var entries []models.FeedEntry

	lines := strings.Split(content, "\n")
	now := time.Now()

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip common comment / metadata prefixes shared by all formats.
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") ||
			strings.HasPrefix(line, "//") {
			continue
		}

		var ipStr string

		switch format {
		case "ip_port":
			// Format: IP:PORT (plain or with comments after whitespace)
			// Strip any trailing comment first.
			if idx := strings.IndexAny(line, " \t#"); idx != -1 {
				line = strings.TrimSpace(line[:idx])
			}
			if strings.Contains(line, "://") {
				parsed, err := url.Parse(line)
				if err != nil || parsed.Host == "" {
					continue
				}
				line = parsed.Host
			}
			addr, _, err := iputil.ParseIPPort(line)
			if err != nil {
				continue
			}
			ipStr = addr.String()

		case "cidr_comments":
			// Format: CIDR ; comment  (Spamhaus DROP/EDROP)
			parts := strings.SplitN(line, ";", 2)
			ipStr = strings.TrimSpace(parts[0])

		case "netset":
			// FireHOL .netset: optionally has metadata lines like "Name= ..."
			// or lines starting with a digit/colon which are CIDR/IP entries.
			// Lines that don't start with a digit or ':' (IPv6) are skipped.
			first := line[0]
			if first != ':' && (first < '0' || first > '9') {
				continue
			}
			// Strip inline comments
			if idx := strings.IndexByte(line, '#'); idx != -1 {
				line = strings.TrimSpace(line[:idx])
			}
			ipStr = line

		case "plain_comments":
			// One IP/CIDR per line, # comments already filtered above.
			// Strip inline comments and trailing whitespace.
			if idx := strings.IndexByte(line, '#'); idx != -1 {
				line = strings.TrimSpace(line[:idx])
				if line == "" {
					continue
				}
			}
			ipStr = line

		case "hostfile":
			// URLhaus hostfile format: "127.0.0.1\tmalware.example.com"
			// The first field is always 127.0.0.1 (loopback redirect).
			// The second field is the malware domain — not an IP we can
			// store in the reputation DB directly.
			// For IP blocking we use the ipblocklist endpoint instead;
			// this case is kept only to avoid silent data loss and emits
			// nothing (the caller should use plain_comments format with
			// the correct URL — see urlhaus feed in feeds.yaml).
			continue

		case "url":
			parsed, err := url.Parse(line)
			if err != nil {
				continue
			}
			host := parsed.Hostname()
			if host == "" {
				continue
			}
			ipStr = host

		default:
			// plain: one IP or CIDR per line, no comments expected but
			// we already stripped # lines above.
			ipStr = line
		}

		ipStr = strings.TrimSpace(ipStr)
		if ipStr == "" {
			continue
		}

		// Try to parse as IP or prefix
		addr, prefix, isPrefix, err := iputil.ParseIPOrPrefix(ipStr)
		if err != nil {
			continue
		}

		sourceID := feedConfig.Name
		if sourceName != "" {
			sourceID += "/" + sourceName
		}
		entry := models.FeedEntry{
			Source:     sourceID,
			ThreatType: feedConfig.ThreatType,
			Confidence: feedConfig.Confidence,
			Weight:     feedConfig.Weight,
			FetchedAt:  now,
		}

		if isPrefix {
			entry.Prefix = prefix
			entry.IPString = prefix.String()
		} else {
			entry.IP = addr
			entry.IPString = addr.String()
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// storeEntries stores parsed entries to the database
func (i *Ingestor) storeEntries(ctx context.Context, entries []models.FeedEntry) error {
	_, err := i.storeEntriesWithCount(ctx, entries)
	return err
}

// storeEntriesWithCount converts FeedEntry slice to DB rows and persists them
// in batches of 5 000. Returns the number of rows actually inserted.
func (i *Ingestor) storeEntriesWithCount(ctx context.Context, entries []models.FeedEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	now := time.Now()
	dbEntries := make([]database.IPReputationEntry, 0, len(entries))

	for _, entry := range entries {
		var ipStart, ipEnd string
		var cidr *string

		if entry.Prefix.IsValid() {
			ipStart, ipEnd = database.IPRangeFromPrefix(entry.Prefix)
			cidrStr := entry.Prefix.String()
			cidr = &cidrStr
		} else if entry.IP.IsValid() {
			ipStart, ipEnd = database.IPRangeFromAddr(entry.IP)
		} else {
			continue
		}

		dbEntries = append(dbEntries, database.IPReputationEntry{
			IPStart:    ipStart,
			IPEnd:      ipEnd,
			CIDR:       cidr,
			Source:     entry.Source,
			ThreatType: entry.ThreatType,
			Confidence: entry.Confidence,
			Weight:     entry.Weight,
			FirstSeen:  now,
			LastSeen:   now,
		})
	}

	if len(dbEntries) == 0 {
		return 0, nil
	}

	const batchSize = 5000
	totalInserted := 0

	for start := 0; start < len(dbEntries); start += batchSize {
		end := start + batchSize
		if end > len(dbEntries) {
			end = len(dbEntries)
		}

		batch := dbEntries[start:end]

		if i.db != nil {
			inserted, err := i.db.InsertReputationBatch(ctx, batch)
			if err != nil {
				return totalInserted, fmt.Errorf("batch insert failed: %w", err)
			}
			totalInserted += inserted
		} else {
			return totalInserted, fmt.Errorf("reputation storage is not configured")
		}
	}

	return totalInserted, nil
}

// processWhitelists fetches static and remote allowlists and stores each source
// atomically. It deliberately keeps private ranges: those are also rejected at
// the API boundary, while the MMDB override protects internal/offline users.
func (i *Ingestor) processWhitelists(ctx context.Context) (int, error) {
	if !i.feedsConfig.Whitelist.Enabled {
		return 0, nil
	}

	total := 0
	var syncErrors []error
	for _, source := range i.feedsConfig.Whitelist.Sources {
		values := append([]string(nil), source.Ranges...)
		if source.URL != "" {
			body, err := i.fetchText(ctx, source.URL)
			if err != nil {
				syncErrors = append(syncErrors, fmt.Errorf("whitelist %s: %w", source.Name, err))
				continue
			}
			values = append(values, strings.Split(body, "\n")...)
		}

		entries, err := parseWhitelistEntries(source.Name, values)
		if err != nil {
			syncErrors = append(syncErrors, err)
			continue
		}
		if i.db != nil {
			stored, err := i.db.ReplaceWhitelistSource(ctx, source.Name, entries)
			if err != nil {
				syncErrors = append(syncErrors, err)
				continue
			}
			total += stored
		} else {
			total += len(entries)
		}
	}
	return total, errors.Join(syncErrors...)
}

func (i *Ingestor) fetchText(ctx context.Context, sourceURL string) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= i.config.Ingestor.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("User-Agent", i.config.Ingestor.UserAgent)
		resp, err := i.httpClient.Do(req)
		if err == nil {
			const maxWhitelistBytes = 10 * 1024 * 1024
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxWhitelistBytes+1))
			resp.Body.Close()
			switch {
			case resp.StatusCode != http.StatusOK:
				lastErr = fmt.Errorf("unexpected status code %d", resp.StatusCode)
			case readErr != nil:
				lastErr = fmt.Errorf("read response: %w", readErr)
			case len(body) > maxWhitelistBytes:
				lastErr = fmt.Errorf("response exceeds %d bytes", maxWhitelistBytes)
			default:
				return string(body), nil
			}
		} else {
			lastErr = err
		}

		if attempt < i.config.Ingestor.MaxRetries {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(i.config.Ingestor.RetryDelay):
			}
		}
	}
	return "", fmt.Errorf("fetch failed after retries: %w", lastErr)
}

func parseWhitelistEntries(source string, values []string) ([]database.WhitelistEntry, error) {
	deduplicated := make(map[string]database.WhitelistEntry)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(value, "#") || strings.HasPrefix(value, ";") {
			continue
		}
		if index := strings.IndexAny(value, " \t#;"); index >= 0 {
			value = strings.TrimSpace(value[:index])
		}
		addr, prefix, isPrefix, err := iputil.ParseIPOrPrefix(value)
		if err != nil {
			continue
		}
		if !isPrefix {
			bits := 128
			if addr.Is4() {
				bits = 32
			}
			prefix = netip.PrefixFrom(addr, bits)
		}
		prefix = prefix.Masked()
		start, end := database.IPRangeFromPrefix(prefix)
		cidr := prefix.String()
		deduplicated[cidr] = database.WhitelistEntry{
			IPStart: start, IPEnd: end, CIDR: &cidr, Name: source,
			Source: source, Permanent: true,
		}
	}
	if len(deduplicated) == 0 {
		return nil, fmt.Errorf("whitelist %s returned no valid IP ranges", source)
	}
	entries := make([]database.WhitelistEntry, 0, len(deduplicated))
	for _, entry := range deduplicated {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool {
		return *entries[left].CIDR < *entries[right].CIDR
	})
	return entries, nil
}

// FetchFeed manually fetches a single feed
func (i *Ingestor) FetchFeed(ctx context.Context, feedName string) error {
	feed, ok := i.feedsConfig.GetFeedByName(feedName)
	if !ok {
		return fmt.Errorf("feed not found: %s", feedName)
	}

	_, _, err := i.processFeedWithStats(ctx, feedName, feed)
	return err
}

func (i *Ingestor) storeSourceSnapshot(ctx context.Context, entries []models.FeedEntry, expiry time.Time) (int, error) {
	if i.db == nil {
		return 0, fmt.Errorf("reputation storage is not configured")
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("empty source snapshot")
	}
	rows := make([]database.IPReputationEntry, 0, len(entries))
	for _, entry := range entries {
		row := database.IPReputationEntry{Source: entry.Source, ThreatType: entry.ThreatType, Confidence: entry.Confidence, Weight: entry.Weight}
		if entry.Prefix.IsValid() {
			row.IPStart, row.IPEnd = database.IPRangeFromPrefix(entry.Prefix)
			value := entry.Prefix.String()
			row.CIDR = &value
		} else if entry.IP.IsValid() {
			row.IPStart, row.IPEnd = database.IPRangeFromAddr(entry.IP)
		} else {
			return 0, fmt.Errorf("invalid parsed entry")
		}
		rows = append(rows, row)
	}
	return i.db.ReplaceReputationSource(ctx, entries[0].Source, rows, expiry)
}
