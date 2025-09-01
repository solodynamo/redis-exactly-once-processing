package phase1

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"

	"redis-timeout-tracking-poc/pkg/config"
	"redis-timeout-tracking-poc/pkg/metrics"
	"redis-timeout-tracking-poc/pkg/models"
)

type TimeoutManager struct {
	rdb     *redis.Client
	config  *config.Config
	logger  *logrus.Logger
	metrics *metrics.Metrics
}

func NewTimeoutManager(rdb *redis.Client, config *config.Config, logger *logrus.Logger, metrics *metrics.Metrics) *TimeoutManager {
	return &TimeoutManager{
		rdb:     rdb,
		config:  config,
		logger:  logger,
		metrics: metrics,
	}
}

// TrackAgentMessage starts tracking timeout for a conversation
func (tm *TimeoutManager) TrackAgentMessage(ctx context.Context, agentMsg models.AgentMessage) error {
	start := time.Now()
	defer func() {
		tm.metrics.RedisOperationDuration.WithLabelValues("track_agent_message").Observe(time.Since(start).Seconds())
	}()

	timestamp := agentMsg.Timestamp.UnixMilli()

	// Use Redis pipeline for atomic operations
	pipe := tm.rdb.Pipeline()

	// Add to waiting conversations sorted set
	pipe.ZAdd(ctx, WaitingConversationsKey, &redis.Z{
		Score:  float64(timestamp),
		Member: agentMsg.ConversationID,
	})

	// Clear any existing notification state
	pipe.HDel(ctx, NotificationStatesKey, agentMsg.ConversationID)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		tm.logger.WithError(err).WithField("conversation_id", agentMsg.ConversationID).Error("Failed to track agent message")
		return fmt.Errorf("failed to track agent message: %w", err)
	}

	tm.logger.WithFields(logrus.Fields{
		"conversation_id": agentMsg.ConversationID,
		"agent_id":        agentMsg.AgentID,
		"timestamp":       agentMsg.Timestamp,
	}).Debug("Started tracking conversation timeout")

	return nil
}

// ClearTimeout removes timeout tracking when customer responds
func (tm *TimeoutManager) ClearTimeout(ctx context.Context, customerResp models.CustomerResponse) error {
	start := time.Now()
	defer func() {
		tm.metrics.RedisOperationDuration.WithLabelValues("clear_timeout").Observe(time.Since(start).Seconds())
	}()

	// Use Redis pipeline for atomic operations
	pipe := tm.rdb.Pipeline()

	// Remove from waiting conversations
	pipe.ZRem(ctx, WaitingConversationsKey, customerResp.ConversationID)

	// Clear notification state
	pipe.HDel(ctx, NotificationStatesKey, customerResp.ConversationID)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		tm.logger.WithError(err).WithField("conversation_id", customerResp.ConversationID).Error("Failed to clear timeout")
		return fmt.Errorf("failed to clear timeout: %w", err)
	}

	tm.logger.WithFields(logrus.Fields{
		"conversation_id": customerResp.ConversationID,
		"customer_id":     customerResp.CustomerID,
		"timestamp":       customerResp.Timestamp,
	}).Debug("Cleared conversation timeout")

	return nil
}

// GetWaitingConversationsCount returns the current number of waiting conversations
func (tm *TimeoutManager) GetWaitingConversationsCount(ctx context.Context) (int64, error) {
	start := time.Now()
	defer func() {
		tm.metrics.RedisOperationDuration.WithLabelValues("get_waiting_count").Observe(time.Since(start).Seconds())
	}()

	count, err := tm.rdb.ZCard(ctx, WaitingConversationsKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get waiting conversations count: %w", err)
	}

	return count, nil
}

// GetNotificationState returns the current notification level for a conversation
func (tm *TimeoutManager) GetNotificationState(ctx context.Context, conversationID string) (int, error) {
	start := time.Now()
	defer func() {
		tm.metrics.RedisOperationDuration.WithLabelValues("get_notification_state").Observe(time.Since(start).Seconds())
	}()

	levelStr, err := tm.rdb.HGet(ctx, NotificationStatesKey, conversationID).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, nil // No notification sent yet
		}
		return 0, fmt.Errorf("failed to get notification state: %w", err)
	}

	level, err := strconv.Atoi(levelStr)
	if err != nil {
		return 0, fmt.Errorf("invalid notification level format: %w", err)
	}

	return level, nil
}

// CleanupExpiredConversations removes conversations that have been waiting too long
func (tm *TimeoutManager) CleanupExpiredConversations(ctx context.Context, maxAge time.Duration) error {
	start := time.Now()
	defer func() {
		tm.metrics.RedisOperationDuration.WithLabelValues("cleanup_expired").Observe(time.Since(start).Seconds())
	}()

	cutoff := time.Now().Add(-maxAge).UnixMilli()

	// Remove conversations older than maxAge
	removed, err := tm.rdb.ZRemRangeByScore(ctx, WaitingConversationsKey, "0", fmt.Sprintf("%d", cutoff)).Result()
	if err != nil {
		return fmt.Errorf("failed to cleanup expired conversations: %w", err)
	}

	if removed > 0 {
		tm.logger.WithFields(logrus.Fields{
			"removed_count": removed,
			"max_age":       maxAge,
		}).Info("Cleaned up expired conversations")
	}

	return nil
}
