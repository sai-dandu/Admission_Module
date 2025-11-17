package utils

import (
	"encoding/json"
	"net/http"
)

// StandardResponse represents a standard API response structure
type StandardResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SendJSON writes a JSON response with the given status code
// This is the base function used by all response helpers
func SendJSON(w http.ResponseWriter, statusCode int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(data)
}

// SendSuccess sends a success response with data
func SendSuccess(w http.ResponseWriter, statusCode int, message string, data interface{}) {
	response := StandardResponse{
		Status:  "success",
		Message: message,
		Data:    data,
	}
	SendJSON(w, statusCode, response)
}

// SendError sends an error response
func SendError(w http.ResponseWriter, statusCode int, message string) {
	response := StandardResponse{
		Status: "error",
		Error:  message,
	}
	SendJSON(w, statusCode, response)
}

// Deprecated: Use SendSuccess instead. Response structure has changed.
// Old function kept for reference
func SuccessResponse(w http.ResponseWriter, message string, data interface{}) {
	SendSuccess(w, http.StatusOK, message, data)
}

// Deprecated: Use SendError instead
func ErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	SendError(w, statusCode, message)
}
