package models

import "time"

// ConversationTimeout represents a conversation waiting for customer response
type ConversationTimeout struct {
	ConversationID   string    `json:"conversation_id"`
	AgentMessageTime time.Time `json:"agent_message_time"`
	Level            int       `json:"level"` // 0, 1, 2, 3 (0 = no notification sent yet)
}

// TimeoutEvent represents a timeout event for Phase 2 stream processing
type TimeoutEvent struct {
	ConversationID   string    `json:"conversation_id"`
	Level            int       `json:"level"`
	AgentMessageTime time.Time `json:"agent_message_time"`
	DetectedAt       time.Time `json:"detected_at"`
	Attempt          int       `json:"attempt"`
}

// NotificationLevel represents the escalation levels
type NotificationLevel int

const (
	NoNotification NotificationLevel = 0
	Level1         NotificationLevel = 1 // N seconds
	Level2         NotificationLevel = 2 // 2N seconds
	Level3         NotificationLevel = 3 // 3N seconds
)

// AgentMessage represents an agent message event
type AgentMessage struct {
	ConversationID string    `json:"conversation_id"`
	AgentID        string    `json:"agent_id"`
	MessageID      string    `json:"message_id"`
	Timestamp      time.Time `json:"timestamp"`
}

// CustomerResponse represents a customer response event
type CustomerResponse struct {
	ConversationID string    `json:"conversation_id"`
	CustomerID     string    `json:"customer_id"`
	MessageID      string    `json:"message_id"`
	Timestamp      time.Time `json:"timestamp"`
}
