package phase2

import (
	"context"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"redis-timeout-tracking-poc/pkg/config"
	"redis-timeout-tracking-poc/pkg/metrics"
	"redis-timeout-tracking-poc/pkg/models"
	"redis-timeout-tracking-poc/pkg/phase1"
)

func setupTestRedisPhase2(t *testing.T) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   2, // Use different test database for Phase 2
	})

	ctx := context.Background()
	err := rdb.Ping(ctx).Err()
	require.NoError(t, err, "Redis should be available for testing")

	// Clean up test data
	rdb.FlushDB(ctx)

	return rdb
}

func TestStreamProducer_CreateConsumerGroup(t *testing.T) {
	rdb := setupTestRedisPhase2(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 5000,
		CheckIntervalMS:   100,
		PodID:             "test-producer",
		ConsumerGroupName: "test-processors",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	producer := NewStreamProducer(rdb, cfg, logger, metrics)

	ctx := context.Background()
	err := producer.createConsumerGroup(ctx)
	assert.NoError(t, err)

	// Verify consumer group exists
	groups, err := rdb.XInfoGroups(ctx, TimeoutEventsStream).Result()
	assert.NoError(t, err)
	assert.Len(t, groups, 1)
	assert.Equal(t, cfg.ConsumerGroupName, groups[0].Name)
}

func TestStreamProducer_PublishTimeoutEvent(t *testing.T) {
	rdb := setupTestRedisPhase2(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 5000,
		PodID:             "test-producer",
		ConsumerGroupName: "test-processors",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	producer := NewStreamProducer(rdb, cfg, logger, metrics)

	ctx := context.Background()
	err := producer.createConsumerGroup(ctx)
	require.NoError(t, err)

	conversationID := "test_conv_123"
	level := 1
	startTime := time.Now().Add(-1 * time.Minute).UnixMilli()

	err = producer.publishTimeoutEvent(ctx, conversationID, level, startTime)
	assert.NoError(t, err)

	// Verify message was added to stream
	messages, err := rdb.XRange(ctx, TimeoutEventsStream, "-", "+").Result()
	assert.NoError(t, err)
	assert.Len(t, messages, 1)

	message := messages[0]
	assert.Equal(t, conversationID, message.Values["conversation_id"])
	assert.Equal(t, "1", message.Values["level"])
}

func TestStreamConsumer_ProcessMessage(t *testing.T) {
	rdb := setupTestRedisPhase2(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 5000,
		PodID:             "test-consumer",
		ConsumerGroupName: "test-processors",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	consumer := NewStreamConsumer(rdb, cfg, logger, metrics)

	ctx := context.Background()

	// Create consumer group first
	err := rdb.XGroupCreateMkStream(ctx, TimeoutEventsStream, cfg.ConsumerGroupName, "$").Err()
	require.NoError(t, err)

	// Add a test message to the stream
	now := time.Now()
	streamArgs := &redis.XAddArgs{
		Stream: TimeoutEventsStream,
		Values: map[string]interface{}{
			"conversation_id":    "test_conv_123",
			"level":              "2",
			"agent_message_time": now.Add(-2 * time.Minute).UnixMilli(),
			"detected_at":        now.UnixMilli(),
			"attempt":            "1",
		},
	}

	_, err = rdb.XAdd(ctx, streamArgs).Result()
	require.NoError(t, err)

	// Read and process the message
	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    cfg.ConsumerGroupName,
		Consumer: consumer.consumerName,
		Streams:  []string{TimeoutEventsStream, ">"},
		Count:    1,
		Block:    100 * time.Millisecond,
	}).Result()

	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)

	message := streams[0].Messages[0]
	consumer.processMessage(ctx, message)

	// Verify message was acknowledged
	pending, err := rdb.XPending(ctx, TimeoutEventsStream, cfg.ConsumerGroupName).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), pending.Count)
}

func TestIntegration_Phase2_EndToEnd(t *testing.T) {
	rdb := setupTestRedisPhase2(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 1000, // 1 second for fast testing
		CheckIntervalMS:   100,  // 100ms check interval
		PodID:             "test-integration",
		ConsumerGroupName: "test-processors",
		LeaderElectionTTL: 5,
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	// Create timeout manager
	timeoutManager := phase1.NewTimeoutManager(rdb, cfg, logger, metrics)

	// Create stream producer and consumer
	producer := NewStreamProducer(rdb, cfg, logger, metrics)
	consumer := NewStreamConsumer(rdb, cfg, logger, metrics)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start components
	err := producer.Start(ctx)
	require.NoError(t, err)
	defer producer.Stop()

	err = consumer.Start(ctx)
	require.NoError(t, err)
	defer consumer.Stop()

	// Track an agent message
	agentMsg := models.AgentMessage{
		ConversationID: "integration_test_conv",
		AgentID:        "agent_123",
		MessageID:      "msg_456",
		Timestamp:      time.Now().Add(-2 * time.Second), // 2 seconds ago
	}

	err = timeoutManager.TrackAgentMessage(ctx, agentMsg)
	require.NoError(t, err)

	// Wait for timeout detection and stream processing
	time.Sleep(2 * time.Second)

	// Check that stream has messages
	length, err := rdb.XLen(ctx, TimeoutEventsStream).Result()
	assert.NoError(t, err)
	assert.Greater(t, length, int64(0))

	// Check notification state was updated
	level, err := timeoutManager.GetNotificationState(ctx, agentMsg.ConversationID)
	assert.NoError(t, err)
	assert.Equal(t, 1, level) // Should have sent level 1 notification
}
