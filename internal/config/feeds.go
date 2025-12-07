package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// FeedsConfig holds all feed configurations
type FeedsConfig struct {
	Feeds     map[string]FeedConfig `mapstructure:"feeds"`
	Formats   map[string]Format     `mapstructure:"formats"`
	Whitelist WhitelistConfig       `mapstructure:"whitelist"`
}

// FeedConfig holds configuration for a single feed
type FeedConfig struct {
	Enabled     bool           `mapstructure:"enabled"`
	Name        string         `mapstructure:"name"`
	Description string         `mapstructure:"description"`
	ThreatType  string         `mapstructure:"threat_type"`
	Confidence  float64        `mapstructure:"confidence"`
	Weight      int            `mapstructure:"weight"`
	Schedule    string         `mapstructure:"schedule"`
	Sources     []SourceConfig `mapstructure:"sources"`
}

// SourceConfig holds configuration for a feed source
type SourceConfig struct {
	URL    string `mapstructure:"url"`
	Format string `mapstructure:"format"`
	Name   string `mapstructure:"name"`
}

// Format defines how to parse a feed
type Format struct {
	Description   string `mapstructure:"description"`
	CommentPrefix string `mapstructure:"comment_prefix"`
	Separator     string `mapstructure:"separator"`
}

// WhitelistConfig holds whitelist configuration
type WhitelistConfig struct {
	Enabled bool                    `mapstructure:"enabled"`
	Sources []WhitelistSourceConfig `mapstructure:"sources"`
}

// WhitelistSourceConfig holds whitelist source configuration
type WhitelistSourceConfig struct {
	Name   string   `mapstructure:"name"`
	URL    string   `mapstructure:"url"`
	Ranges []string `mapstructure:"ranges"`
}

// LoadFeeds loads feeds configuration from file
func LoadFeeds(configPath string) (*FeedsConfig, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read feeds config: %w", err)
	}

	var cfg FeedsConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal feeds config: %w", err)
	}

	return &cfg, nil
}

// GetEnabledFeeds returns only enabled feeds
func (fc *FeedsConfig) GetEnabledFeeds() map[string]FeedConfig {
	enabled := make(map[string]FeedConfig)
	for name, feed := range fc.Feeds {
		if feed.Enabled {
			enabled[name] = feed
		}
	}
	return enabled
}

// GetFeedByName returns a specific feed by name
func (fc *FeedsConfig) GetFeedByName(name string) (FeedConfig, bool) {
	feed, ok := fc.Feeds[name]
	return feed, ok
}

// GetFormat returns a format parser configuration
func (fc *FeedsConfig) GetFormat(name string) (Format, bool) {
	format, ok := fc.Formats[name]
	return format, ok
}

// ParseSchedule converts cron schedule to duration for simple intervals
func ParseSchedule(schedule string) (time.Duration, error) {
	switch schedule {
	case "@hourly":
		return time.Hour, nil
	case "@daily":
		return 24 * time.Hour, nil
	case "@weekly":
		return 7 * 24 * time.Hour, nil
	default:
		// Return 0 for cron expressions (need to be handled by cron parser)
		return 0, nil
	}
}
