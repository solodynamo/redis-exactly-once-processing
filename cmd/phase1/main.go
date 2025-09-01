package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"redis-timeout-tracking-poc/pkg/config"
	"redis-timeout-tracking-poc/pkg/metrics"
	"redis-timeout-tracking-poc/pkg/phase1"
	redisClient "redis-timeout-tracking-poc/pkg/redis"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Setup logger
	logger := logrus.New()
	if level, err := logrus.ParseLevel(cfg.LogLevel); err == nil {
		logger.SetLevel(level)
	}
	logger.SetFormatter(&logrus.JSONFormatter{})

	logger.WithField("pod_id", cfg.PodID).Info("Starting Phase 1 timeout tracking service")

	// Initialize metrics
	metrics := metrics.NewMetrics()

	// Connect to Redis
	redisConfig := redisClient.DefaultConnectionConfig()
	redisConfig.URL = cfg.RedisURL

	redis, err := redisClient.NewClient(redisConfig, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to connect to Redis")
	}
	defer redis.Close()

	// Create Phase 1 service
	service := phase1.NewService(redis.GetRedisClient(), cfg, logger, metrics)

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start service
	if err := service.Start(ctx); err != nil {
		logger.WithError(err).Fatal("Failed to start service")
	}

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	<-sigCh
	logger.Info("Received shutdown signal")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := service.Stop(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error during service shutdown")
	}

	logger.Info("Phase 1 service shutdown complete")
}
