package constants

import "time"

// Timeout notification level multipliers
// These define when notifications are sent relative to the base timeout interval
const (
	// TimeoutLevel1Multiplier - First notification after N seconds
	TimeoutLevel1Multiplier = 1

	// TimeoutLevel2Multiplier - Second notification after 2N seconds
	TimeoutLevel2Multiplier = 2

	// TimeoutLevel3Multiplier - Third notification after 3N seconds
	TimeoutLevel3Multiplier = 3
)

// Default timeout configuration values
const (
	// DefaultTimeoutIntervalSeconds - Default base timeout interval in seconds
	DefaultTimeoutIntervalSeconds = 30

	// DefaultLeaderElectionTTLSeconds - Default leader election TTL in seconds
	DefaultLeaderElectionTTLSeconds = 10

	// DefaultLeaderElectionIntervalSeconds - Default leader election check interval
	DefaultLeaderElectionIntervalSeconds = 5

	// DefaultCleanupIntervalSeconds - Default cleanup interval for expired conversations
	DefaultCleanupIntervalSeconds = 60
)

// Timeout levels as constants for better code readability
const (
	TimeoutLevelNone = 0
	TimeoutLevel1    = 1
	TimeoutLevel2    = 2
	TimeoutLevel3    = 3
	MaxTimeoutLevel  = TimeoutLevel3
)

// Redis key prefixes and names
const (
	WaitingConversationsKey = "waiting_conversations"
	NotificationStatesKey   = "notification_states"
	LeaderElectionKey       = "timeout:leader"
	MetricsKey              = "metrics:timeouts"
	TimeoutEventsStream     = "timeout_events"
)

// Configuration environment variable names
const (
	EnvTimeoutInterval         = "TIMEOUT_INTERVAL_SECONDS"
	EnvTimeoutLevel1Multiplier = "TIMEOUT_LEVEL_1_MULTIPLIER"
	EnvTimeoutLevel2Multiplier = "TIMEOUT_LEVEL_2_MULTIPLIER"
	EnvTimeoutLevel3Multiplier = "TIMEOUT_LEVEL_3_MULTIPLIER"
	EnvLeaderElectionTTL       = "LEADER_ELECTION_TTL_SECONDS"
	EnvLeaderElectionInterval  = "LEADER_ELECTION_INTERVAL_SECONDS"
	EnvCleanupInterval         = "CLEANUP_INTERVAL_SECONDS"
)

// Helper functions for time conversions
func SecondsToMilliseconds(seconds int) int64 {
	return int64(seconds * 1000)
}

func SecondsToDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

// GetTimeoutThresholdMS returns the timeout threshold in milliseconds for a given level
func GetTimeoutThresholdMS(baseIntervalMS int64, level int) int64 {
	switch level {
	case TimeoutLevel1:
		return baseIntervalMS * TimeoutLevel1Multiplier
	case TimeoutLevel2:
		return baseIntervalMS * TimeoutLevel2Multiplier
	case TimeoutLevel3:
		return baseIntervalMS * TimeoutLevel3Multiplier
	default:
		return baseIntervalMS
	}
}
