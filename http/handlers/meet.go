package handlers

import (
	"admission-module/db"
	"admission-module/http/services"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func ScheduleMeet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StudentID int `json:"student_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Get student email
	var email string
	err := db.DB.QueryRow("SELECT email FROM student_lead WHERE id = $1", req.StudentID).Scan(&email)
	if err != nil {
		http.Error(w, "Student not found", http.StatusNotFound)
		return
	}

	// Schedule meet
	meetLink, err := services.ScheduleMeet(email)
	if err != nil {
		// Log detailed error server-side and return a helpful message to the client
		log.Printf("ScheduleMeet error for student %d: %v", req.StudentID, err)
		http.Error(w, "Error scheduling meet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Update DB
	_, err = db.DB.Exec("UPDATE student_lead SET meet_link = $1, application_status = 'MEETING_SCHEDULED' WHERE id = $2", meetLink, req.StudentID)
	if err != nil {
		http.Error(w, "Error updating lead", http.StatusInternalServerError)
		return
	}

	// Send email
	err = services.SendEmail(email, "Google Meet Scheduled", "Your meet link: "+meetLink)
	if err != nil {
		// Log error but don't fail
	}

	// Publish to Kafka
	evt := map[string]interface{}{
		"event":        "meeting.scheduled",
		"student_id":   req.StudentID,
		"email":        email,
		"meet_link":    meetLink,
		"status":       "scheduled",
		"scheduled_at": time.Now().Unix(),
	}
	evtJSON, _ := json.Marshal(evt)
	services.Publish("meetings", fmt.Sprintf("student-%d", req.StudentID), string(evtJSON))

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"meet_link": meetLink})
}
