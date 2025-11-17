package handlers

// import (
// 	"encoding/json"
// 	"log"
// 	"net/http"
// )

// AssignCounsellor is DEPRECATED. Use LeadService.assignCounsellorTx() instead.
// Counselor assignment is now automatic during lead creation.
// This handler is kept for backward compatibility only.
// func AssignCounsellor(w http.ResponseWriter, r *http.Request) {
// 	var req struct {
// 		LeadSource string `json:"lead_source"`
// 	}

// 	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
// 		respondError(w, "Invalid request", http.StatusBadRequest)
// 		return
// 	}

// 	log.Printf("Warning: AssignCounsellor endpoint is deprecated. Counselor assignment is handled during lead creation.")

// 	// Return deprecation notice
// 	w.WriteHeader(http.StatusOK)
// 	json.NewEncoder(w).Encode(map[string]string{
// 		"message": "Deprecated: Counselor assignment is now automatic during lead creation via /create-lead or /upload-leads endpoints.",
// 	})
// }
