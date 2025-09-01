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

const (
	LeaderKey               = "timeout:leader"
	WaitingConversationsKey = "waiting_conversations"
	NotificationStatesKey   = "notification_states"
	MetricsKey              = "metrics:timeouts"
)

type LeaderElection struct {
	rdb      *redis.Client
	config   *config.Config
	logger   *logrus.Logger
	metrics  *metrics.Metrics
	isLeader bool
	stopCh   chan struct{}
}

func NewLeaderElection(rdb *redis.Client, config *config.Config, logger *logrus.Logger, metrics *metrics.Metrics) *LeaderElection {
	return &LeaderElection{
		rdb:     rdb,
		config:  config,
		logger:  logger,
		metrics: metrics,
		stopCh:  make(chan struct{}),
	}
}

func (le *LeaderElection) Start(ctx context.Context) error {
	le.logger.Info("Starting leader election process")

	// Try to become leader immediately
	go le.leaderElectionLoop(ctx)

	// Start timeout checking loop
	go le.timeoutCheckLoop(ctx)

	return nil
}

func (le *LeaderElection) Stop() {
	close(le.stopCh)
	if le.isLeader {
		le.resignLeadership(context.Background())
	}
}

func (le *LeaderElection) IsLeader() bool {
	// Always verify leadership status against Redis
	ctx := context.Background()
	currentLeader, err := le.rdb.Get(ctx, LeaderKey).Result()
	if err != nil {
		le.isLeader = false
		return false
	}

	isActualLeader := currentLeader == le.config.PodID
	if le.isLeader != isActualLeader {
		// Update local state to match Redis truth
		le.isLeader = isActualLeader
		if isActualLeader {
			le.logger.Info("Confirmed leadership from Redis")
		} else {
			le.logger.Info("Leadership lost - not in Redis")
		}
	}

	return le.isLeader
}

func (le *LeaderElection) leaderElectionLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-le.stopCh:
			return
		case <-ticker.C:
			le.tryBecomeLeader(ctx)
		}
	}
}

func (le *LeaderElection) tryBecomeLeader(ctx context.Context) {
	start := time.Now()
	defer func() {
		le.metrics.LeaderElectionDuration.Observe(time.Since(start).Seconds())
	}()

	result := le.rdb.SetArgs(ctx, LeaderKey, le.config.PodID, redis.SetArgs{
		Mode: "NX",
		TTL:  le.config.LeaderElectionTTLDuration(),
	})

	if result.Err() != nil {
		le.logger.WithError(result.Err()).Error("Failed to attempt leader election")
		return
	}

	if result.Val() == "OK" {
		if !le.isLeader {
			le.logger.Info("Became leader")
			le.metrics.TimeoutLeaderChanges.Inc()
			le.isLeader = true
		}
		// Renew leadership
		le.renewLeadership(ctx)
	} else {
		// Failed to acquire leadership, check if we think we're leader but aren't
		if le.isLeader {
			// Double-check by reading the current leader from Redis
			currentLeader, err := le.rdb.Get(ctx, LeaderKey).Result()
			if err != nil || currentLeader != le.config.PodID {
				le.logger.Info("Lost leadership")
				le.isLeader = false
			}
		}
	}
}

func (le *LeaderElection) renewLeadership(ctx context.Context) {
	// Check if we're still the leader and extend TTL
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("EXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`

	result := le.rdb.Eval(ctx, script, []string{LeaderKey}, le.config.PodID, le.config.LeaderElectionTTL)
	if result.Err() != nil {
		le.logger.WithError(result.Err()).Error("Failed to renew leadership")
		le.isLeader = false
		return
	}

	if result.Val().(int64) == 0 {
		le.logger.Warn("Leadership renewal failed - no longer leader")
		le.isLeader = false
	}
}

func (le *LeaderElection) resignLeadership(ctx context.Context) {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`

	result := le.rdb.Eval(ctx, script, []string{LeaderKey}, le.config.PodID)
	if result.Err() != nil {
		le.logger.WithError(result.Err()).Error("Failed to resign leadership")
	} else {
		le.logger.Info("Resigned leadership")
	}
	le.isLeader = false
}

func (le *LeaderElection) timeoutCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(le.config.CheckInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-le.stopCh:
			return
		case <-ticker.C:
			if le.isLeader {
				le.checkTimeouts(ctx)
			}
		}
	}
}

func (le *LeaderElection) checkTimeouts(ctx context.Context) {
	start := time.Now()
	defer func() {
		le.metrics.TimeoutCheckDuration.Observe(time.Since(start).Seconds())
	}()

	now := time.Now().UnixMilli()

	// Get all waiting conversations
	conversations, err := le.rdb.ZRangeByScoreWithScores(ctx, WaitingConversationsKey, &redis.ZRangeBy{
		Min: "0",
		Max: fmt.Sprintf("%d", now),
	}).Result()

	if err != nil {
		le.logger.WithError(err).Error("Failed to get waiting conversations")
		return
	}

	le.metrics.WaitingConversationsCount.Set(float64(len(conversations)))

	for _, conv := range conversations {
		conversationID := conv.Member.(string)
		startTime := int64(conv.Score)

		le.processConversationTimeout(ctx, conversationID, startTime, now)
	}
}

func (le *LeaderElection) processConversationTimeout(ctx context.Context, conversationID string, startTime, now int64) {
	waitTime := now - startTime
	timeoutInterval := le.config.TimeoutIntervalMS

	// Get current notification level
	currentLevelStr, err := le.rdb.HGet(ctx, NotificationStatesKey, conversationID).Result()
	if err != nil && err != redis.Nil {
		le.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to get notification state")
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

	// Send notification and update state
	if err := le.sendNotification(ctx, conversationID, newLevel, startTime); err != nil {
		le.logger.WithError(err).WithFields(logrus.Fields{
			"conversation_id": conversationID,
			"level":           newLevel,
		}).Error("Failed to send notification")
		return
	}

	// Update notification state
	if err := le.rdb.HSet(ctx, NotificationStatesKey, conversationID, newLevel).Err(); err != nil {
		le.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to update notification state")
		return
	}

	le.metrics.TimeoutNotificationsSent.WithLabelValues(fmt.Sprintf("level%d", newLevel)).Inc()

	le.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"level":           newLevel,
		"wait_time_ms":    waitTime,
	}).Info("Sent timeout notification")
}

func (le *LeaderElection) sendNotification(ctx context.Context, conversationID string, level int, startTime int64) error {
	// In a real implementation, this would call your notification service
	// For POC, we'll just log and update metrics

	notification := models.TimeoutEvent{
		ConversationID:   conversationID,
		Level:            level,
		AgentMessageTime: time.UnixMilli(startTime),
		DetectedAt:       time.Now(),
		Attempt:          1,
	}

	le.logger.WithFields(logrus.Fields{
		"conversation_id": notification.ConversationID,
		"level":           notification.Level,
		"detected_at":     notification.DetectedAt,
	}).Info("Sending timeout notification")

	// TODO: Replace with actual notification service call
	// Example: notificationService.SendTimeoutAlert(notification)

	return nil
}
