package handlers

import (
	"admission-module/db"
	"admission-module/services"
	"encoding/json"
	"fmt"
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

	// REQUIREMENT: Check if registration fee is PAID before allowing interview scheduling
	var regPaymentStatus string
	err = db.DB.QueryRow("SELECT status FROM registration_payment WHERE student_id = $1", req.StudentID).Scan(&regPaymentStatus)
	if err != nil {
		http.Error(w, "Registration payment record not found. Please complete registration fee payment first", http.StatusBadRequest)
		return
	}
	if regPaymentStatus != "PAID" {
		http.Error(w, fmt.Sprintf("Interview cannot be scheduled. Registration payment status is %s. Please complete registration fee payment first", regPaymentStatus), http.StatusBadRequest)
		return
	}

	// Schedule meet
	meetLink, err := services.ScheduleMeet(req.StudentID, email)
	if err != nil {
		http.Error(w, "Error scheduling meet: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Note: meet_link is already stored in ScheduleMeet(), just update application_status
	_, err = db.DB.Exec("UPDATE student_lead SET application_status = 'MEETING_SCHEDULED', updated_at = CURRENT_TIMESTAMP WHERE id = $1", req.StudentID)
	if err != nil {
		http.Error(w, "Error updating lead", http.StatusInternalServerError)
		return
	}

	// Send email
	_ = services.SendEmail(email, "Google Meet Scheduled", "Your meet link: "+meetLink)

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
