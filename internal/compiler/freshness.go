package compiler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/lfrfrfr/beon-ipquality/internal/config"
	internalmetrics "github.com/lfrfrfr/beon-ipquality/internal/metrics"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
	"github.com/robfig/cron/v3"
)

// FreshnessBudget allows one scheduled interval plus processing/publication
// delay. Different schedules must not all be represented as a one-hour SLA.
func FreshnessBudget(schedule string, now time.Time, compile, reload, fetch time.Duration) (time.Duration, error) {
	parsed, err := cron.ParseStandard(schedule)
	if err != nil {
		return 0, err
	}
	next := parsed.Next(now)
	second := parsed.Next(next)
	if next.IsZero() || second.IsZero() {
		return 0, fmt.Errorf("schedule has no future run")
	}
	return second.Sub(next) + compile + reload + fetch, nil
}

func (c *Compiler) fetchFeedFreshness(ctx context.Context, now time.Time) ([]models.FeedFreshness, error) {
	rows, err := c.db.Query(ctx, `SELECT feed_name,COALESCE(source_url,''),max(created_at) FILTER (WHERE status='success'),(array_agg(status ORDER BY created_at DESC,id DESC))[1] FROM feed_fetch_history GROUP BY feed_name,source_url`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := make(map[string]models.FeedFreshness)
	for rows.Next() {
		var feed, source string
		var state models.FeedFreshness
		if err := rows.Scan(&feed, &source, &state.LastSuccess, &state.LatestStatus); err != nil {
			return nil, err
		}
		history[feed+"\n"+source] = state
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	feeds, err := config.LoadFeeds(c.config.MMDB.FeedsPath)
	if err != nil {
		return nil, err
	}
	result := make([]models.FeedFreshness, 0)
	for id, feed := range feeds.GetEnabledFeeds() {
		budget, err := FreshnessBudget(feed.Schedule, now, c.config.MMDB.CompileInterval, c.config.MMDB.ReloadInterval, c.config.Ingestor.HTTPTimeout*time.Duration(c.config.Ingestor.MaxRetries+1)+c.config.Ingestor.RetryDelay*time.Duration(c.config.Ingestor.MaxRetries))
		if err != nil {
			return nil, fmt.Errorf("feed %s schedule: %w", id, err)
		}
		for _, source := range feed.Sources {
			state := history[id+"\n"+source.URL]
			state.Feed, state.Source, state.MaxAgeSeconds = id, source.Name, budget.Seconds()
			if state.LastSuccess != nil {
				internalmetrics.FeedLastSuccess.WithLabelValues(id, source.Name).Set(float64(state.LastSuccess.Unix()))
			}
			if state.LatestStatus == "" {
				state.LatestStatus = "never_fetched"
			}
			result = append(result, state)
		}
	}
	// Stable metadata order makes diagnostics and hashes reproducible.
	sort.Slice(result, func(i, j int) bool {
		if result[i].Feed != result[j].Feed {
			return result[i].Feed < result[j].Feed
		}
		return result[i].Source < result[j].Source
	})
	return result, nil
}
