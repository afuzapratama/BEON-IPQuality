package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
	"github.com/lfrfrfr/beon-ipquality/pkg/models"
)

// Cache interface defines the caching operations
type Cache interface {
	Get(ctx context.Context, ip string) (*models.IPCheckResult, error)
	Set(ctx context.Context, ip string, result *models.IPCheckResult) error
	Delete(ctx context.Context, ip string) error
	Clear(ctx context.Context) error
	Stats(ctx context.Context) (*CacheStats, error)
	Close() error
}

// CacheStats holds cache statistics
type CacheStats struct {
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	HitRate    float64 `json:"hit_rate"`
	Keys       int64   `json:"keys"`
	MemoryUsed int64   `json:"memory_used_bytes"`
}

// RedisCache implements Cache interface using Redis
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
	prefix string
	hits   int64
	misses int64
}

// Config holds Redis cache configuration
type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int
	TTL      time.Duration
	Prefix   string
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(cfg Config) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	ttl := cfg.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute // Default TTL
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "ipq:"
	}

	logger.Info(fmt.Sprintf("Connected to Redis at %s:%d", cfg.Host, cfg.Port))

	return &RedisCache{
		client: client,
		ttl:    ttl,
		prefix: prefix,
	}, nil
}

// key generates the cache key for an IP
func (c *RedisCache) key(ip string) string {
	return c.prefix + ip
}

// Get retrieves a cached result for an IP
func (c *RedisCache) Get(ctx context.Context, ip string) (*models.IPCheckResult, error) {
	data, err := c.client.Get(ctx, c.key(ip)).Bytes()
	if err != nil {
		if err == redis.Nil {
			c.misses++
			return nil, nil // Cache miss
		}
		return nil, err
	}

	var result models.IPCheckResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached result: %w", err)
	}

	c.hits++
	result.Cached = true
	return &result, nil
}

// Set stores a result in the cache
func (c *RedisCache) Set(ctx context.Context, ip string, result *models.IPCheckResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	return c.client.Set(ctx, c.key(ip), data, c.ttl).Err()
}

// Delete removes a cached result
func (c *RedisCache) Delete(ctx context.Context, ip string) error {
	return c.client.Del(ctx, c.key(ip)).Err()
}

// Clear removes all cached results
func (c *RedisCache) Clear(ctx context.Context) error {
	// Use SCAN to find all keys with our prefix and delete them
	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, c.prefix+"*", 1000).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	c.hits = 0
	c.misses = 0
	return nil
}

// Stats returns cache statistics
func (c *RedisCache) Stats(ctx context.Context) (*CacheStats, error) {
	info, err := c.client.Info(ctx, "memory", "stats").Result()
	if err != nil {
		return nil, err
	}

	// Count keys with our prefix
	var keyCount int64
	var cursor uint64
	for {
		keys, nextCursor, err := c.client.Scan(ctx, cursor, c.prefix+"*", 1000).Result()
		if err != nil {
			return nil, err
		}
		keyCount += int64(len(keys))
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	stats := &CacheStats{
		Hits:   c.hits,
		Misses: c.misses,
		Keys:   keyCount,
	}

	total := c.hits + c.misses
	if total > 0 {
		stats.HitRate = float64(c.hits) / float64(total) * 100
	}

	// Parse memory usage from info string (simplified)
	_ = info // TODO: Parse memory info if needed

	return stats, nil
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// NoOpCache implements Cache interface but does nothing (for when caching is disabled)
type NoOpCache struct{}

// NewNoOpCache creates a no-op cache
func NewNoOpCache() *NoOpCache {
	return &NoOpCache{}
}

func (c *NoOpCache) Get(ctx context.Context, ip string) (*models.IPCheckResult, error) {
	return nil, nil
}

func (c *NoOpCache) Set(ctx context.Context, ip string, result *models.IPCheckResult) error {
	return nil
}

func (c *NoOpCache) Delete(ctx context.Context, ip string) error {
	return nil
}

func (c *NoOpCache) Clear(ctx context.Context) error {
	return nil
}

func (c *NoOpCache) Stats(ctx context.Context) (*CacheStats, error) {
	return &CacheStats{}, nil
}

func (c *NoOpCache) Close() error {
	return nil
}
