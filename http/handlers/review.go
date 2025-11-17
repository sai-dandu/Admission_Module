package handlers

import (
	"admission-module/db"
	"admission-module/http/services"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func ApplicationAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StudentID int    `json:"student_id"`
		Status    string `json:"status"` // ACCEPTED or REJECTED
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Status != "ACCEPTED" && req.Status != "REJECTED" {
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	// Get student details
	var name, email string
	err := db.DB.QueryRow("SELECT name, email FROM leads WHERE id = $1", req.StudentID).Scan(&name, &email)
	if err != nil {
		http.Error(w, "Student not found", http.StatusNotFound)
		return
	}

	// Update status
	_, err = db.DB.Exec("UPDATE leads SET application_status = $1 WHERE id = $2", req.Status, req.StudentID)
	if err != nil {
		http.Error(w, "Error updating lead", http.StatusInternalServerError)
		return
	}

	if req.Status == "ACCEPTED" {
		// Generate offer letter
		pdfPath, err := services.GenerateOfferLetter(name, email)
		if err != nil {
			http.Error(w, "Error generating offer letter", http.StatusInternalServerError)
			return
		}

		// Send email with PDF
		err = services.SendEmail(email, "Offer Letter", "Congratulations! Your offer letter is attached.", pdfPath)
		if err != nil {
			http.Error(w, "Error sending email", http.StatusInternalServerError)
			return
		}

		// Publish email.sent event to Kafka (async, best-effort)
		go func() {
			evt := map[string]interface{}{
				"event":      "email.sent",
				"to":         email,
				"subject":    "Offer Letter",
				"status":     "sent",
				"student_id": req.StudentID,
				"ts":         time.Now().Unix(),
			}
			if err := services.Publish("emails", email, evt); err != nil {
				fmt.Printf("Warning: failed to publish email.sent event: %v\n", err)
			}
		}()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("Application accepted and offer letter sent to %s", name),
		})
	} else {
		// Send rejection email
		err = services.SendEmail(email, "Application Status", "We regret to inform you that your application has been rejected.")
		if err != nil {
			// Log error but still return success
			fmt.Printf("Warning: failed to send rejection email: %v\n", err)
		}

		// Publish email.sent event to Kafka (async, best-effort)
		go func() {
			evt := map[string]interface{}{
				"event":      "email.sent",
				"to":         email,
				"subject":    "Application Status",
				"status":     "sent",
				"student_id": req.StudentID,
				"ts":         time.Now().Unix(),
			}
			if err := services.Publish("emails", email, evt); err != nil {
				fmt.Printf("Warning: failed to publish email.sent event: %v\n", err)
			}
		}()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": fmt.Sprintf("Application rejected and notification sent to %s", name),
		})
	}
}
