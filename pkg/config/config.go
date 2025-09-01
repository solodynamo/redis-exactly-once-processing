package config

import (
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	RedisURL          string
	TimeoutIntervalMS int64
	CheckIntervalMS   int64
	LeaderElectionTTL int
	PodID             string
	Port              string
	Phase2Mode        bool
	ConsumerGroupName string
	LogLevel          string
	MetricsPort       string
}

func Load() *Config {
	config := &Config{
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		TimeoutIntervalMS: getEnvInt64("TIMEOUT_INTERVAL_MS", 30000),
		CheckIntervalMS:   getEnvInt64("CHECK_INTERVAL_MS", 1000),
		LeaderElectionTTL: getEnvInt("LEADER_ELECTION_TTL", 10),
		PodID:             getEnv("POD_ID", generatePodID()),
		Port:              getEnv("PORT", "8080"),
		Phase2Mode:        getEnvBool("PHASE2_MODE", false),
		ConsumerGroupName: getEnv("CONSUMER_GROUP_NAME", "timeout-processors"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		MetricsPort:       getEnv("METRICS_PORT", "9090"),
	}

	return config
}

func (c *Config) TimeoutInterval() time.Duration {
	return time.Duration(c.TimeoutIntervalMS) * time.Millisecond
}

func (c *Config) CheckInterval() time.Duration {
	return time.Duration(c.CheckIntervalMS) * time.Millisecond
}

func (c *Config) LeaderElectionTTLDuration() time.Duration {
	return time.Duration(c.LeaderElectionTTL) * time.Second
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func generatePodID() string {
	hostname, err := os.Hostname()
	if err != nil {
		return uuid.New().String()
	}
	return hostname + "-" + uuid.New().String()[:8]
}
