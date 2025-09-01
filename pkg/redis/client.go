package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type Client struct {
	rdb    *redis.Client
	logger *logrus.Logger
}

type ConnectionConfig struct {
	URL                string
	MaxRetries         int
	MinRetryBackoff    time.Duration
	MaxRetryBackoff    time.Duration
	DialTimeout        time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	PoolSize           int
	MinIdleConns       int
	MaxConnAge         time.Duration
	PoolTimeout        time.Duration
	IdleTimeout        time.Duration
	IdleCheckFrequency time.Duration
}

func NewClient(config ConnectionConfig, logger *logrus.Logger) (*Client, error) {
	opt, err := redis.ParseURL(config.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Apply connection configuration
	opt.MaxRetries = config.MaxRetries
	opt.MinRetryBackoff = config.MinRetryBackoff
	opt.MaxRetryBackoff = config.MaxRetryBackoff
	opt.DialTimeout = config.DialTimeout
	opt.ReadTimeout = config.ReadTimeout
	opt.WriteTimeout = config.WriteTimeout
	opt.PoolSize = config.PoolSize
	opt.MinIdleConns = config.MinIdleConns
	opt.MaxConnAge = config.MaxConnAge
	opt.PoolTimeout = config.PoolTimeout
	opt.IdleTimeout = config.IdleTimeout
	opt.IdleCheckFrequency = config.IdleCheckFrequency

	rdb := redis.NewClient(opt)

	client := &Client{
		rdb:    rdb,
		logger: logger,
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Successfully connected to Redis")
	return client, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) GetRedisClient() *redis.Client {
	return c.rdb
}

// DefaultConnectionConfig returns a production-ready Redis configuration
func DefaultConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		MaxRetries:         3,
		MinRetryBackoff:    8 * time.Millisecond,
		MaxRetryBackoff:    512 * time.Millisecond,
		DialTimeout:        5 * time.Second,
		ReadTimeout:        3 * time.Second,
		WriteTimeout:       3 * time.Second,
		PoolSize:           10,
		MinIdleConns:       5,
		MaxConnAge:         30 * time.Minute,
		PoolTimeout:        4 * time.Second,
		IdleTimeout:        5 * time.Minute,
		IdleCheckFrequency: 1 * time.Minute,
	}
}
