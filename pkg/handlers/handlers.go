package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"redis-timeout-tracking-poc/pkg/models"
	"redis-timeout-tracking-poc/pkg/phase1"
)

type Handler struct {
	timeoutManager *phase1.TimeoutManager
	logger         *logrus.Logger
	isLeaderFunc   func() bool
}

func NewHandler(timeoutManager *phase1.TimeoutManager, logger *logrus.Logger, isLeaderFunc func() bool) *Handler {
	return &Handler{
		timeoutManager: timeoutManager,
		logger:         logger,
		isLeaderFunc:   isLeaderFunc,
	}
}

func (h *Handler) AgentMessage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	conversationID := vars["id"]

	if conversationID == "" {
		http.Error(w, "Missing conversation ID", http.StatusBadRequest)
		return
	}

	var request struct {
		AgentID   string    `json:"agent_id"`
		MessageID string    `json:"message_id"`
		Timestamp time.Time `json:"timestamp,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Timestamp.IsZero() {
		request.Timestamp = time.Now()
	}

	agentMsg := models.AgentMessage{
		ConversationID: conversationID,
		AgentID:        request.AgentID,
		MessageID:      request.MessageID,
		Timestamp:      request.Timestamp,
	}

	if err := h.timeoutManager.TrackAgentMessage(r.Context(), agentMsg); err != nil {
		h.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to track agent message")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":         true,
		"conversation_id": conversationID,
		"tracked_at":      request.Timestamp,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	h.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"agent_id":        request.AgentID,
	}).Debug("Tracked agent message")
}

func (h *Handler) CustomerResponse(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	conversationID := vars["id"]

	if conversationID == "" {
		http.Error(w, "Missing conversation ID", http.StatusBadRequest)
		return
	}

	var request struct {
		CustomerID string    `json:"customer_id"`
		MessageID  string    `json:"message_id"`
		Timestamp  time.Time `json:"timestamp,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Timestamp.IsZero() {
		request.Timestamp = time.Now()
	}

	customerResp := models.CustomerResponse{
		ConversationID: conversationID,
		CustomerID:     request.CustomerID,
		MessageID:      request.MessageID,
		Timestamp:      request.Timestamp,
	}

	if err := h.timeoutManager.ClearTimeout(r.Context(), customerResp); err != nil {
		h.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to clear timeout")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":         true,
		"conversation_id": conversationID,
		"cleared_at":      request.Timestamp,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	h.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"customer_id":     request.CustomerID,
	}).Debug("Cleared conversation timeout")
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	count, err := h.timeoutManager.GetWaitingConversationsCount(r.Context())
	if err != nil {
		http.Error(w, "Health check failed", http.StatusServiceUnavailable)
		return
	}

	response := map[string]interface{}{
		"status":                "healthy",
		"is_leader":             h.isLeaderFunc(),
		"waiting_conversations": count,
		"timestamp":             time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	count, err := h.timeoutManager.GetWaitingConversationsCount(r.Context())
	if err != nil {
		http.Error(w, "Failed to get status", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"is_leader":             h.isLeaderFunc(),
		"waiting_conversations": count,
		"timestamp":             time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
