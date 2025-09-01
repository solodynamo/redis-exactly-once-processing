package phase2

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

type StreamConsumer struct {
	rdb          *redis.Client
	config       *config.Config
	logger       *logrus.Logger
	metrics      *metrics.Metrics
	consumerName string
	stopCh       chan struct{}
}

func NewStreamConsumer(rdb *redis.Client, config *config.Config, logger *logrus.Logger, metrics *metrics.Metrics) *StreamConsumer {
	consumerName := fmt.Sprintf("consumer-%s", config.PodID)

	return &StreamConsumer{
		rdb:          rdb,
		config:       config,
		logger:       logger,
		metrics:      metrics,
		consumerName: consumerName,
		stopCh:       make(chan struct{}),
	}
}

func (sc *StreamConsumer) Start(ctx context.Context) error {
	sc.logger.WithField("consumer_name", sc.consumerName).Info("Starting stream consumer")

	// Start consuming messages
	go sc.consumeLoop(ctx)

	// Start pending messages recovery
	go sc.pendingMessagesRecovery(ctx)

	sc.logger.Info("Stream consumer started successfully")
	return nil
}

func (sc *StreamConsumer) Stop() {
	close(sc.stopCh)
}

func (sc *StreamConsumer) consumeLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-sc.stopCh:
			return
		default:
			sc.consumeMessages(ctx)
		}
	}
}

func (sc *StreamConsumer) consumeMessages(ctx context.Context) {
	start := time.Now()

	// Read messages from stream
	streams, err := sc.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    sc.config.ConsumerGroupName,
		Consumer: sc.consumerName,
		Streams:  []string{TimeoutEventsStream, ">"},
		Count:    10,
		Block:    1 * time.Second,
	}).Result()

	if err != nil {
		if err != redis.Nil {
			sc.logger.WithError(err).Error("Failed to read from stream")
		}
		return
	}

	for _, stream := range streams {
		for _, message := range stream.Messages {
			sc.processMessage(ctx, message)
		}
	}

	if len(streams) > 0 {
		sc.metrics.StreamProcessingDuration.Observe(time.Since(start).Seconds())
	}
}

func (sc *StreamConsumer) processMessage(ctx context.Context, message redis.XMessage) {
	start := time.Now()
	defer func() {
		sc.metrics.RedisOperationDuration.WithLabelValues("process_message").Observe(time.Since(start).Seconds())
	}()

	// Parse message
	event, err := sc.parseTimeoutEvent(message)
	if err != nil {
		sc.logger.WithError(err).WithField("message_id", message.ID).Error("Failed to parse timeout event")
		sc.metrics.StreamMessagesProcessed.WithLabelValues("parse_error").Inc()
		// Acknowledge message to prevent reprocessing
		sc.acknowledgeMessage(ctx, message.ID)
		return
	}

	// Process the timeout notification
	if err := sc.sendNotification(ctx, event); err != nil {
		sc.logger.WithError(err).WithFields(logrus.Fields{
			"conversation_id": event.ConversationID,
			"level":           event.Level,
			"message_id":      message.ID,
		}).Error("Failed to send notification")
		sc.metrics.StreamMessagesProcessed.WithLabelValues("notification_error").Inc()
		// Don't acknowledge - let it retry
		return
	}

	// Acknowledge successful processing
	if err := sc.acknowledgeMessage(ctx, message.ID); err != nil {
		sc.logger.WithError(err).WithField("message_id", message.ID).Error("Failed to acknowledge message")
		return
	}

	sc.metrics.StreamMessagesProcessed.WithLabelValues("success").Inc()
	sc.metrics.TimeoutNotificationsSent.WithLabelValues(fmt.Sprintf("level%d", event.Level)).Inc()

	sc.logger.WithFields(logrus.Fields{
		"conversation_id": event.ConversationID,
		"level":           event.Level,
		"message_id":      message.ID,
	}).Debug("Successfully processed timeout event")
}

func (sc *StreamConsumer) parseTimeoutEvent(message redis.XMessage) (*models.TimeoutEvent, error) {
	event := &models.TimeoutEvent{}

	// Extract fields from message
	if convID, ok := message.Values["conversation_id"].(string); ok {
		event.ConversationID = convID
	} else {
		return nil, fmt.Errorf("missing or invalid conversation_id")
	}

	if levelStr, ok := message.Values["level"].(string); ok {
		if level, err := strconv.Atoi(levelStr); err == nil {
			event.Level = level
		} else {
			return nil, fmt.Errorf("invalid level format: %w", err)
		}
	} else {
		return nil, fmt.Errorf("missing or invalid level")
	}

	if agentTimeStr, ok := message.Values["agent_message_time"].(string); ok {
		if agentTime, err := strconv.ParseInt(agentTimeStr, 10, 64); err == nil {
			event.AgentMessageTime = time.UnixMilli(agentTime)
		} else {
			return nil, fmt.Errorf("invalid agent_message_time format: %w", err)
		}
	} else {
		return nil, fmt.Errorf("missing or invalid agent_message_time")
	}

	if detectedAtStr, ok := message.Values["detected_at"].(string); ok {
		if detectedAt, err := strconv.ParseInt(detectedAtStr, 10, 64); err == nil {
			event.DetectedAt = time.UnixMilli(detectedAt)
		} else {
			return nil, fmt.Errorf("invalid detected_at format: %w", err)
		}
	} else {
		return nil, fmt.Errorf("missing or invalid detected_at")
	}

	if attemptStr, ok := message.Values["attempt"].(string); ok {
		if attempt, err := strconv.Atoi(attemptStr); err == nil {
			event.Attempt = attempt
		} else {
			return nil, fmt.Errorf("invalid attempt format: %w", err)
		}
	} else {
		event.Attempt = 1 // Default
	}

	return event, nil
}

func (sc *StreamConsumer) sendNotification(ctx context.Context, event *models.TimeoutEvent) error {
	// In a real implementation, this would call your notification service
	// For POC, we'll simulate the notification sending

	sc.logger.WithFields(logrus.Fields{
		"conversation_id": event.ConversationID,
		"level":           event.Level,
		"detected_at":     event.DetectedAt,
		"attempt":         event.Attempt,
	}).Info("Sending timeout notification via stream consumer")

	// Simulate notification service call
	time.Sleep(10 * time.Millisecond) // Simulate API call latency

	// TODO: Replace with actual notification service call
	// Example:
	// if err := notificationService.SendTimeoutAlert(ctx, event); err != nil {
	//     return fmt.Errorf("notification service error: %w", err)
	// }

	return nil
}

func (sc *StreamConsumer) acknowledgeMessage(ctx context.Context, messageID string) error {
	return sc.rdb.XAck(ctx, TimeoutEventsStream, sc.config.ConsumerGroupName, messageID).Err()
}

func (sc *StreamConsumer) pendingMessagesRecovery(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second) // Check for pending messages every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-sc.stopCh:
			return
		case <-ticker.C:
			sc.processPendingMessages(ctx)
		}
	}
}

func (sc *StreamConsumer) processPendingMessages(ctx context.Context) {
	// Get pending messages for this consumer
	pending, err := sc.rdb.XPending(ctx, TimeoutEventsStream, sc.config.ConsumerGroupName).Result()
	if err != nil {
		sc.logger.WithError(err).Error("Failed to get pending messages")
		return
	}

	if pending.Count == 0 {
		return
	}

	sc.logger.WithField("pending_count", pending.Count).Info("Processing pending messages")

	// Claim messages that have been pending for more than 1 minute
	minIdleTime := 1 * time.Minute
	messages, _, err := sc.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   TimeoutEventsStream,
		Group:    sc.config.ConsumerGroupName,
		Consumer: sc.consumerName,
		MinIdle:  minIdleTime,
		Count:    10,
		Start:    "0-0",
	}).Result()

	if err != nil {
		sc.logger.WithError(err).Error("Failed to auto-claim pending messages")
		return
	}

	for _, message := range messages {
		sc.processMessage(ctx, message)
	}
}
