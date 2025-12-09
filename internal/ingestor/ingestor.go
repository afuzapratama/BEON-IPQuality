package ingestor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	"github.com/lfrfrfr/beon-ipquality/internal/database"
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
		cron:        cron.New(), // Standard 5-field cron format (minute, hour, day, month, weekday)
	}, nil
}

// Start starts the ingestor service
func (i *Ingestor) Start(ctx context.Context) error {
	i.mu.Lock()
	if i.running {
		i.mu.Unlock()
		return fmt.Errorf("ingestor already running")
	}
	i.running = true
	i.mu.Unlock()

	// Schedule feeds
	enabledFeeds := i.feedsConfig.GetEnabledFeeds()
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
	i.cron.Start()

	// Run initial fetch for all feeds
	logger.Info("Running initial fetch for all feeds...")
	i.runAllFeeds(ctx)

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

	i.cron.Stop()
	i.wg.Wait()
	i.running = false
}

// RunOnce runs all feeds once and returns statistics (for --once mode)
func (i *Ingestor) RunOnce(ctx context.Context) (totalFeeds, totalEntries, totalStored int, err error) {
	enabledFeeds := i.feedsConfig.GetEnabledFeeds()
	totalFeeds = len(enabledFeeds)

	// Results tracking with mutex for thread safety
	var mu sync.Mutex
	var errors []error

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
				errors = append(errors, feedErr)
			}
			mu.Unlock()
		}(name, feed)
	}

	// Wait for all feeds to complete
	i.wg.Wait()

	if len(errors) > 0 {
		err = fmt.Errorf("%d feed(s) had errors", len(errors))
	}

	return
}

// runAllFeeds runs all enabled feeds
func (i *Ingestor) runAllFeeds(ctx context.Context) {
	enabledFeeds := i.feedsConfig.GetEnabledFeeds()

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

// processFeed processes a single feed
func (i *Ingestor) processFeed(ctx context.Context, feedName string, feedConfig config.FeedConfig) {
	logger.Info(fmt.Sprintf("Processing feed: %s", feedName))
	startTime := time.Now()

	totalEntries := 0

	for _, source := range feedConfig.Sources {
		select {
		case <-ctx.Done():
			return
		default:
		}

		entries, err := i.fetchSource(ctx, source, feedConfig)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to fetch source %s/%s: %v", feedName, source.Name, err))
			continue
		}

		// Store entries
		if err := i.storeEntries(entries); err != nil {
			logger.Error(fmt.Sprintf("Failed to store entries for %s/%s: %v", feedName, source.Name, err))
			continue
		}

		totalEntries += len(entries)
		logger.Info(fmt.Sprintf("Fetched %d entries from %s/%s", len(entries), feedName, source.Name))
	}

	logger.Info(fmt.Sprintf("Completed feed %s: %d total entries in %v", feedName, totalEntries, time.Since(startTime)))
}

// processFeedWithStats processes a single feed and returns statistics
func (i *Ingestor) processFeedWithStats(ctx context.Context, feedName string, feedConfig config.FeedConfig) (totalEntries, totalStored int, err error) {
	// Print progress to stdout for --once mode
	fmt.Printf("\033[0;34m[*]\033[0m Processing feed: %s\n", feedName)
	startTime := time.Now()

	for _, source := range feedConfig.Sources {
		select {
		case <-ctx.Done():
			return totalEntries, totalStored, ctx.Err()
		default:
		}

		entries, fetchErr := i.fetchSource(ctx, source, feedConfig)
		if fetchErr != nil {
			fmt.Printf("\033[0;31m[✗]\033[0m   Source %s: %v\n", source.Name, fetchErr)
			continue
		}

		totalEntries += len(entries)

		// Store entries and get count
		stored, storeErr := i.storeEntriesWithCount(entries)
		if storeErr != nil {
			fmt.Printf("\033[0;31m[✗]\033[0m   Source %s: store error: %v\n", source.Name, storeErr)
			continue
		}

		totalStored += stored
		fmt.Printf("\033[0;32m[✓]\033[0m   %s/%s: fetched %d, stored %d\n", feedName, source.Name, len(entries), stored)
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\033[0;32m[✓]\033[0m Completed %s: %d entries in %v\n", feedName, totalEntries, elapsed.Round(time.Millisecond))
	
	return totalEntries, totalStored, nil
}

// fetchSource fetches and parses a single source
func (i *Ingestor) fetchSource(ctx context.Context, source config.SourceConfig, feedConfig config.FeedConfig) ([]models.FeedEntry, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", source.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", i.config.Ingestor.UserAgent)

	// Retry logic
	var resp *http.Response
	for attempt := 0; attempt <= i.config.Ingestor.MaxRetries; attempt++ {
		resp, err = i.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		if attempt < i.config.Ingestor.MaxRetries {
			time.Sleep(i.config.Ingestor.RetryDelay)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch after %d retries: %w", i.config.Ingestor.MaxRetries, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse based on format
	return i.parseContent(string(body), source.Format, feedConfig)
}

// parseContent parses the content based on format
func (i *Ingestor) parseContent(content, format string, feedConfig config.FeedConfig) ([]models.FeedEntry, error) {
	var entries []models.FeedEntry

	lines := strings.Split(content, "\n")
	now := time.Now()

	// Get format configuration
	formatConfig, _ := i.feedsConfig.GetFormat(format)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Skip comments
		if formatConfig.CommentPrefix != "" && strings.HasPrefix(line, formatConfig.CommentPrefix) {
			continue
		}

		// Also skip common comment prefixes
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}

		var ipStr string

		switch format {
		case "ip_port":
			// Format: IP:PORT
			addr, _, err := iputil.ParseIPPort(line)
			if err != nil {
				continue
			}
			ipStr = addr.String()

		case "cidr_comments":
			// Format: CIDR ; comment
			parts := strings.SplitN(line, ";", 2)
			ipStr = strings.TrimSpace(parts[0])

		default:
			// Plain format - just the IP or CIDR
			ipStr = line
		}

		// Try to parse as IP or prefix
		addr, prefix, isPrefix, err := iputil.ParseIPOrPrefix(ipStr)
		if err != nil {
			continue
		}

		entry := models.FeedEntry{
			Source:     feedConfig.Name,
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
func (i *Ingestor) storeEntries(entries []models.FeedEntry) error {
	if len(entries) == 0 {
		return nil
	}

	// Convert FeedEntry to database entries
	dbEntries := make([]database.IPReputationEntry, 0, len(entries))
	now := time.Now()

	for _, entry := range entries {
		var ipStart, ipEnd string
		var cidr *string

		if entry.Prefix.IsValid() {
			// It's a CIDR prefix
			ipStart, ipEnd = database.IPRangeFromPrefix(entry.Prefix)
			cidrStr := entry.Prefix.String()
			cidr = &cidrStr
		} else if entry.IP.IsValid() {
			// It's a single IP
			ipStart, ipEnd = database.IPRangeFromAddr(entry.IP)
		} else {
			continue
		}

		dbEntry := database.IPReputationEntry{
			IPStart:    ipStart,
			IPEnd:      ipEnd,
			CIDR:       cidr,
			Source:     entry.Source,
			ThreatType: entry.ThreatType,
			Confidence: entry.Confidence,
			Weight:     entry.Weight,
			FirstSeen:  now,
			LastSeen:   now,
		}

		dbEntries = append(dbEntries, dbEntry)
	}

	if len(dbEntries) == 0 {
		return nil
	}

	// Store in batches to avoid memory issues
	batchSize := 5000
	totalInserted := 0

	for start := 0; start < len(dbEntries); start += batchSize {
		end := start + batchSize
		if end > len(dbEntries) {
			end = len(dbEntries)
		}

		batch := dbEntries[start:end]

		if i.db != nil {
			inserted, err := i.db.InsertReputationBatch(context.Background(), batch)
			if err != nil {
				logger.Error(fmt.Sprintf("Failed to insert batch: %v", err))
				continue
			}
			totalInserted += inserted
		} else {
			// No DB connection, just log
			totalInserted += len(batch)
		}
	}

	logger.Info(fmt.Sprintf("Stored %d entries to database", totalInserted))
	return nil
}

// storeEntriesWithCount stores parsed entries and returns count stored
func (i *Ingestor) storeEntriesWithCount(entries []models.FeedEntry) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}

	// Convert FeedEntry to database entries
	dbEntries := make([]database.IPReputationEntry, 0, len(entries))
	now := time.Now()

	for _, entry := range entries {
		var ipStart, ipEnd string
		var cidr *string

		if entry.Prefix.IsValid() {
			// It's a CIDR prefix
			ipStart, ipEnd = database.IPRangeFromPrefix(entry.Prefix)
			cidrStr := entry.Prefix.String()
			cidr = &cidrStr
		} else if entry.IP.IsValid() {
			// It's a single IP
			ipStart, ipEnd = database.IPRangeFromAddr(entry.IP)
		} else {
			continue
		}

		dbEntry := database.IPReputationEntry{
			IPStart:    ipStart,
			IPEnd:      ipEnd,
			CIDR:       cidr,
			Source:     entry.Source,
			ThreatType: entry.ThreatType,
			Confidence: entry.Confidence,
			Weight:     entry.Weight,
			FirstSeen:  now,
			LastSeen:   now,
		}

		dbEntries = append(dbEntries, dbEntry)
	}

	if len(dbEntries) == 0 {
		return 0, nil
	}

	// Store in batches
	batchSize := 5000
	totalInserted := 0

	for start := 0; start < len(dbEntries); start += batchSize {
		end := start + batchSize
		if end > len(dbEntries) {
			end = len(dbEntries)
		}

		batch := dbEntries[start:end]

		if i.db != nil {
			inserted, err := i.db.InsertReputationBatch(context.Background(), batch)
			if err != nil {
				return totalInserted, fmt.Errorf("batch insert failed: %w", err)
			}
			totalInserted += inserted
		} else {
			totalInserted += len(batch)
		}
	}

	return totalInserted, nil
}

// FetchFeed manually fetches a single feed
func (i *Ingestor) FetchFeed(ctx context.Context, feedName string) error {
	feed, ok := i.feedsConfig.GetFeedByName(feedName)
	if !ok {
		return fmt.Errorf("feed not found: %s", feedName)
	}

	i.processFeed(ctx, feedName, feed)
	return nil
}
