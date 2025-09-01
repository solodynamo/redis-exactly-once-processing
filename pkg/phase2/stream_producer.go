package phase2

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"

	"redis-timeout-tracking-poc/pkg/config"
	"redis-timeout-tracking-poc/pkg/metrics"
	"redis-timeout-tracking-poc/pkg/models"
	"redis-timeout-tracking-poc/pkg/phase1"
)

const (
	TimeoutEventsStream = "timeout_events"
)

type StreamProducer struct {
	rdb            *redis.Client
	config         *config.Config
	logger         *logrus.Logger
	metrics        *metrics.Metrics
	leaderElection *phase1.LeaderElection
}

func NewStreamProducer(rdb *redis.Client, config *config.Config, logger *logrus.Logger, metrics *metrics.Metrics) *StreamProducer {
	leaderElection := phase1.NewLeaderElection(rdb, config, logger, metrics)

	return &StreamProducer{
		rdb:            rdb,
		config:         config,
		logger:         logger,
		metrics:        metrics,
		leaderElection: leaderElection,
	}
}

func (sp *StreamProducer) Start(ctx context.Context) error {
	sp.logger.Info("Starting Phase 2 stream producer (leader-only timeout detector)")

	// Create consumer group if it doesn't exist
	if err := sp.createConsumerGroup(ctx); err != nil {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Start leader election
	if err := sp.leaderElection.Start(ctx); err != nil {
		return fmt.Errorf("failed to start leader election: %w", err)
	}

	// Start timeout detection loop (leader only)
	go sp.timeoutDetectionLoop(ctx)

	sp.logger.Info("Stream producer started successfully")
	return nil
}

func (sp *StreamProducer) Stop() {
	sp.leaderElection.Stop()
}

func (sp *StreamProducer) IsLeader() bool {
	return sp.leaderElection.IsLeader()
}

func (sp *StreamProducer) createConsumerGroup(ctx context.Context) error {
	// Create consumer group (idempotent operation)
	err := sp.rdb.XGroupCreateMkStream(ctx, TimeoutEventsStream, sp.config.ConsumerGroupName, "$").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	sp.logger.WithField("consumer_group", sp.config.ConsumerGroupName).Info("Consumer group ready")
	return nil
}

func (sp *StreamProducer) timeoutDetectionLoop(ctx context.Context) {
	ticker := time.NewTicker(sp.config.CheckInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if sp.leaderElection.IsLeader() {
				sp.detectAndPublishTimeouts(ctx)
			}
		}
	}
}

func (sp *StreamProducer) detectAndPublishTimeouts(ctx context.Context) {
	start := time.Now()
	defer func() {
		sp.metrics.TimeoutCheckDuration.Observe(time.Since(start).Seconds())
	}()

	now := time.Now().UnixMilli()

	// Get all waiting conversations
	conversations, err := sp.rdb.ZRangeByScoreWithScores(ctx, phase1.WaitingConversationsKey, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()

	if err != nil {
		sp.logger.WithError(err).Error("Failed to get waiting conversations")
		return
	}

	sp.metrics.WaitingConversationsCount.Set(float64(len(conversations)))

	for _, conv := range conversations {
		conversationID := conv.Member.(string)
		startTime := int64(conv.Score)

		sp.processTimeoutDetection(ctx, conversationID, startTime, now)
	}
}

func (sp *StreamProducer) processTimeoutDetection(ctx context.Context, conversationID string, startTime, now int64) {
	waitTime := now - startTime
	timeoutInterval := sp.config.TimeoutIntervalMS

	// Get current notification level
	currentLevelStr, err := sp.rdb.HGet(ctx, phase1.NotificationStatesKey, conversationID).Result()
	if err != nil && err != redis.Nil {
		sp.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to get notification state")
		return
	}

	currentLevel := 0
	if currentLevelStr != "" {
		if level, err := strconv.Atoi(currentLevelStr); err == nil {
			currentLevel = level
		}
	}

	// Check for timeout levels
	var newLevel int
	if waitTime > timeoutInterval*3 && currentLevel < 3 {
		newLevel = 3
	} else if waitTime > timeoutInterval*2 && currentLevel < 2 {
		newLevel = 2
	} else if waitTime > timeoutInterval && currentLevel < 1 {
		newLevel = 1
	} else {
		return // No new notification needed
	}

	// Publish timeout event to stream
	if err := sp.publishTimeoutEvent(ctx, conversationID, newLevel, startTime); err != nil {
		sp.logger.WithError(err).WithFields(logrus.Fields{
			"conversation_id": conversationID,
			"level":           newLevel,
		}).Error("Failed to publish timeout event")
		return
	}

	// Update notification state to prevent duplicate detection
	if err := sp.rdb.HSet(ctx, phase1.NotificationStatesKey, conversationID, newLevel).Err(); err != nil {
		sp.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to update notification state")
	}

	sp.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"level":           newLevel,
		"wait_time_ms":    waitTime,
	}).Debug("Published timeout event to stream")
}

func (sp *StreamProducer) publishTimeoutEvent(ctx context.Context, conversationID string, level int, startTime int64) error {
	event := models.TimeoutEvent{
		ConversationID:   conversationID,
		Level:            level,
		AgentMessageTime: time.UnixMilli(startTime),
		DetectedAt:       time.Now(),
		Attempt:          1,
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal timeout event: %w", err)
	}

	// Add to Redis stream
	streamArgs := &redis.XAddArgs{
		Stream: TimeoutEventsStream,
		Values: map[string]interface{}{
			"conversation_id":    event.ConversationID,
			"level":              event.Level,
			"agent_message_time": event.AgentMessageTime.UnixMilli(),
			"detected_at":        event.DetectedAt.UnixMilli(),
			"attempt":            event.Attempt,
			"event_data":         string(eventData),
		},
	}

	messageID, err := sp.rdb.XAdd(ctx, streamArgs).Result()
	if err != nil {
		return fmt.Errorf("failed to add message to stream: %w", err)
	}

	sp.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"level":           level,
		"message_id":      messageID,
	}).Debug("Published timeout event to stream")

	return nil
}
