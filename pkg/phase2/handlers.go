package phase2

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"redis-timeout-tracking-poc/pkg/models"
)

func (s *Service) handleAgentMessage(w http.ResponseWriter, r *http.Request) {
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

	if err := s.timeoutManager.TrackAgentMessage(r.Context(), agentMsg); err != nil {
		s.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to track agent message")
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

	s.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"agent_id":        request.AgentID,
	}).Debug("Tracked agent message")
}

func (s *Service) handleCustomerResponse(w http.ResponseWriter, r *http.Request) {
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

	if err := s.timeoutManager.ClearTimeout(r.Context(), customerResp); err != nil {
		s.logger.WithError(err).WithField("conversation_id", conversationID).Error("Failed to clear timeout")
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

	s.logger.WithFields(logrus.Fields{
		"conversation_id": conversationID,
		"customer_id":     request.CustomerID,
	}).Debug("Cleared conversation timeout")
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	count, err := s.timeoutManager.GetWaitingConversationsCount(r.Context())
	if err != nil {
		http.Error(w, "Health check failed", http.StatusServiceUnavailable)
		return
	}

	response := map[string]interface{}{
		"status":                "healthy",
		"is_leader":             s.IsLeader(),
		"waiting_conversations": count,
		"timestamp":             time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Service) handleStatus(w http.ResponseWriter, r *http.Request) {
	count, err := s.timeoutManager.GetWaitingConversationsCount(r.Context())
	if err != nil {
		http.Error(w, "Failed to get status", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"pod_id":                s.config.PodID,
		"is_leader":             s.IsLeader(),
		"waiting_conversations": count,
		"timestamp":             time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
