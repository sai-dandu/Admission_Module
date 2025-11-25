package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"admission-module/logger"
	"admission-module/services"
	"admission-module/utils"
)

// GetDLQMessages retrieves unresolved DLQ messages
// GET /api/dlq/messages?limit=50
func GetDLQMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get limit from query parameter, default to 50
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	messages, err := services.GetDLQMessages(limit)
	if err != nil {
		logger.Error("Error fetching DLQ messages: %v", err)
		utils.SendError(w, http.StatusInternalServerError, "Failed to fetch DLQ messages: "+err.Error())
		return
	}

	utils.SendSuccess(w, http.StatusOK, "DLQ messages retrieved", map[string]interface{}{
		"count": len(messages),
		"data":  messages,
	})
}

// RetryDLQMessage retries processing of a specific DLQ message
// POST /api/dlq/messages/:messageId/retry
func RetryDLQMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messageID := r.URL.Query().Get("id")
	if messageID == "" {
		utils.SendError(w, http.StatusBadRequest, "Missing message ID parameter")
		return
	}

	if err := services.RetryDLQMessage(messageID); err != nil {
		logger.Error("Error retrying DLQ message %s: %v", messageID, err)
		utils.SendError(w, http.StatusInternalServerError, "Failed to retry message: "+err.Error())
		return
	}

	utils.SendSuccess(w, http.StatusOK, "Message retry initiated", map[string]interface{}{
		"messageId": messageID,
	})
}

// ResolveDLQMessage marks a DLQ message as resolved
// POST /api/dlq/messages/:messageId/resolve
func ResolveDLQMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	messageID := r.URL.Query().Get("id")
	if messageID == "" {
		utils.SendError(w, http.StatusBadRequest, "Missing message ID parameter")
		return
	}

	var req struct {
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Notes = "Manually resolved"
	}

	if err := services.ResolveDLQMessage(messageID, req.Notes); err != nil {
		logger.Error("Error resolving DLQ message %s: %v", messageID, err)
		utils.SendError(w, http.StatusInternalServerError, "Failed to resolve message: "+err.Error())
		return
	}

	utils.SendSuccess(w, http.StatusOK, "Message marked as resolved", map[string]interface{}{
		"messageId": messageID,
	})
}

// GetDLQStats retrieves statistics about DLQ messages
// GET /api/dlq/stats
func GetDLQStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := services.GetDLQStats()
	if err != nil {
		logger.Error("Error fetching DLQ statistics: %v", err)
		utils.SendError(w, http.StatusInternalServerError, "Failed to fetch DLQ statistics: "+err.Error())
		return
	}

	utils.SendSuccess(w, http.StatusOK, "DLQ statistics", stats)
}
