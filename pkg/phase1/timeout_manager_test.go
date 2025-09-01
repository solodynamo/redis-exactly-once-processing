package phase1

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"redis-timeout-tracking-poc/pkg/config"
	"redis-timeout-tracking-poc/pkg/metrics"
	"redis-timeout-tracking-poc/pkg/models"
)

func setupTestRedis(t *testing.T) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   1, // Use test database
	})

	// Test connection
	ctx := context.Background()
	err := rdb.Ping(ctx).Err()
	require.NoError(t, err, "Redis should be available for testing")

	// Clean up test data
	rdb.FlushDB(ctx)

	return rdb
}

func TestTimeoutManager_TrackAgentMessage(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 30000,
		PodID:             "test-pod",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	metrics := metrics.NewMetrics()

	tm := NewTimeoutManager(rdb, cfg, logger, metrics)

	ctx := context.Background()
	agentMsg := models.AgentMessage{
		ConversationID: "conv_123",
		AgentID:        "agent_456",
		MessageID:      "msg_789",
		Timestamp:      time.Now(),
	}

	err := tm.TrackAgentMessage(ctx, agentMsg)
	assert.NoError(t, err)

	// Verify conversation is tracked
	score, err := rdb.ZScore(ctx, WaitingConversationsKey, agentMsg.ConversationID).Result()
	assert.NoError(t, err)
	assert.Equal(t, float64(agentMsg.Timestamp.UnixMilli()), score)

	// Verify notification state is cleared
	exists, err := rdb.HExists(ctx, NotificationStatesKey, agentMsg.ConversationID).Result()
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestTimeoutManager_ClearTimeout(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 30000,
		PodID:             "test-pod",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	tm := NewTimeoutManager(rdb, cfg, logger, metrics)

	ctx := context.Background()
	conversationID := "conv_123"

	// First track an agent message
	agentMsg := models.AgentMessage{
		ConversationID: conversationID,
		AgentID:        "agent_456",
		MessageID:      "msg_789",
		Timestamp:      time.Now(),
	}
	err := tm.TrackAgentMessage(ctx, agentMsg)
	require.NoError(t, err)

	// Set a notification state
	err = rdb.HSet(ctx, NotificationStatesKey, conversationID, 1).Err()
	require.NoError(t, err)

	// Clear timeout
	customerResp := models.CustomerResponse{
		ConversationID: conversationID,
		CustomerID:     "customer_123",
		MessageID:      "msg_999",
		Timestamp:      time.Now(),
	}
	err = tm.ClearTimeout(ctx, customerResp)
	assert.NoError(t, err)

	// Verify conversation is removed from waiting list
	_, err = rdb.ZScore(ctx, WaitingConversationsKey, conversationID).Result()
	assert.Error(t, err) // Should return redis.Nil error
	assert.Equal(t, redis.Nil, err)

	// Verify notification state is cleared
	exists, err := rdb.HExists(ctx, NotificationStatesKey, conversationID).Result()
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestTimeoutManager_GetWaitingConversationsCount(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 30000,
		PodID:             "test-pod",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	tm := NewTimeoutManager(rdb, cfg, logger, metrics)

	ctx := context.Background()

	// Initially should be 0
	count, err := tm.GetWaitingConversationsCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Add some conversations
	for i := 0; i < 5; i++ {
		agentMsg := models.AgentMessage{
			ConversationID: fmt.Sprintf("conv_%d", i),
			AgentID:        "agent_456",
			MessageID:      fmt.Sprintf("msg_%d", i),
			Timestamp:      time.Now(),
		}
		err := tm.TrackAgentMessage(ctx, agentMsg)
		require.NoError(t, err)
	}

	// Should now be 5
	count, err = tm.GetWaitingConversationsCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestTimeoutManager_GetNotificationState(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 30000,
		PodID:             "test-pod",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	tm := NewTimeoutManager(rdb, cfg, logger, metrics)

	ctx := context.Background()
	conversationID := "conv_123"

	// Initially should be 0
	level, err := tm.GetNotificationState(ctx, conversationID)
	assert.NoError(t, err)
	assert.Equal(t, 0, level)

	// Set notification level
	err = rdb.HSet(ctx, NotificationStatesKey, conversationID, 2).Err()
	require.NoError(t, err)

	// Should return 2
	level, err = tm.GetNotificationState(ctx, conversationID)
	assert.NoError(t, err)
	assert.Equal(t, 2, level)
}

func TestTimeoutManager_CleanupExpiredConversations(t *testing.T) {
	rdb := setupTestRedis(t)
	defer rdb.Close()

	cfg := &config.Config{
		TimeoutIntervalMS: 30000,
		PodID:             "test-pod",
	}
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	metrics := metrics.NewMetrics()

	tm := NewTimeoutManager(rdb, cfg, logger, metrics)

	ctx := context.Background()

	// Add old and new conversations
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now()

	oldMsg := models.AgentMessage{
		ConversationID: "old_conv",
		AgentID:        "agent_456",
		MessageID:      "old_msg",
		Timestamp:      oldTime,
	}
	newMsg := models.AgentMessage{
		ConversationID: "new_conv",
		AgentID:        "agent_456",
		MessageID:      "new_msg",
		Timestamp:      newTime,
	}

	err := tm.TrackAgentMessage(ctx, oldMsg)
	require.NoError(t, err)
	err = tm.TrackAgentMessage(ctx, newMsg)
	require.NoError(t, err)

	// Cleanup conversations older than 1 hour
	err = tm.CleanupExpiredConversations(ctx, 1*time.Hour)
	assert.NoError(t, err)

	// Old conversation should be removed
	_, err = rdb.ZScore(ctx, WaitingConversationsKey, "old_conv").Result()
	assert.Equal(t, redis.Nil, err)

	// New conversation should remain
	_, err = rdb.ZScore(ctx, WaitingConversationsKey, "new_conv").Result()
	assert.NoError(t, err)
}
