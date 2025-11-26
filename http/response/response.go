package response

import (
	"encoding/json"
	"log"
	"net/http"
)

// StandardResponse represents the standard API response structure
type StandardResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SuccessResponse sends a success response with given status code, message, and data
func SuccessResponse(w http.ResponseWriter, statusCode int, message string, data interface{}) {
	response := StandardResponse{
		Status:  "success",
		Message: message,
		Data:    data,
	}
	SendJSON(w, statusCode, response)
}

// ErrorResponse sends an error response with given status code and error message
func ErrorResponse(w http.ResponseWriter, statusCode int, errorMsg string) {
	response := StandardResponse{
		Status: "error",
		Error:  errorMsg,
	}
	SendJSON(w, statusCode, response)
}

// SendJSON encodes and sends a JSON response
func SendJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}
