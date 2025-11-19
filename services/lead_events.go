package services

import (
	"admission-module/config"
	"admission-module/db"
	"admission-module/models"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// LeadCreatedEvent represents a lead creation event for Kafka
type LeadCreatedEvent struct {
	EventID      string    `json:"event_id"`
	EventType    string    `json:"event_type"` // "lead.created"
	LeadID       int       `json:"lead_id"`
	StudentName  string    `json:"student_name"`
	StudentEmail string    `json:"student_email"`
	StudentPhone string    `json:"student_phone"`
	CounselorID  *int64    `json:"counselor_id,omitempty"`
	LeadSource   string    `json:"lead_source"`
	Timestamp    time.Time `json:"timestamp"`
}

// PublishLeadCreatedEvent publishes a lead created event to Kafka
// This is non-blocking and uses best-effort delivery
// The event will be consumed by background workers to send emails
func PublishLeadCreatedEvent(lead *models.Lead) error {
	event := LeadCreatedEvent{
		EventID:      fmt.Sprintf("lead-%d-%d", lead.ID, time.Now().UnixNano()),
		EventType:    "lead.created",
		LeadID:       lead.ID,
		StudentName:  lead.Name,
		StudentEmail: lead.Email,
		StudentPhone: lead.Phone,
		CounselorID:  lead.CounsellorID,
		LeadSource:   lead.LeadSource,
		Timestamp:    time.Now().UTC(),
	}

	// Publish to Kafka with the lead ID as key for partitioning
	// Non-blocking publish - failure doesn't affect lead creation
	go func() {
		if err := Publish(config.AppConfig.KafkaLeadEventsTopic, fmt.Sprintf("lead-%d", lead.ID), event); err != nil {
			log.Printf("Warning: failed to publish lead.created event to Kafka for lead ID %d: %v", lead.ID, err)
		} else {
			log.Printf("✅ Published lead.created event to Kafka topic '%s' for lead ID %d", config.AppConfig.KafkaLeadEventsTopic, lead.ID)
		}
	}()

	return nil
}

// HandleLeadCreatedEvent processes a lead created event from Kafka
// This is called by a background consumer and sends the welcome emails
func HandleLeadCreatedEvent(eventData []byte) error {
	var event LeadCreatedEvent
	if err := json.Unmarshal(eventData, &event); err != nil {
		return fmt.Errorf("error unmarshaling lead created event: %w", err)
	}

	log.Printf("Processing lead.created event: Lead ID=%d, Student=%s", event.LeadID, event.StudentEmail)

	// Get counselor information if assigned
	if event.CounselorID != nil {
		var counselorName, counselorEmail, counselorPhone string
		query := "SELECT name, email, phone FROM counselor WHERE id = $1"
		err := db.DB.QueryRow(query, *event.CounselorID).Scan(&counselorName, &counselorEmail, &counselorPhone)
		if err != nil {
			log.Printf("Error fetching counselor details for lead ID %d: %v", event.LeadID, err)
			// Continue anyway - send email without counselor details
		} else {
			// Send student welcome email
			if err := SendCounselorAssignmentEmail(event.StudentName, event.StudentEmail, counselorName, counselorEmail, counselorPhone); err != nil {
				log.Printf("Error sending welcome email to student: %v", err)
				return err
			}

			// Send counselor notification email (non-critical)
			if err := SendCounselorAssignmentNotificationEmail(counselorName, counselorEmail, event.StudentName, event.StudentPhone, event.StudentEmail, event.LeadSource); err != nil {
				log.Printf("Error sending counselor notification email: %v", err)
				// Don't return error - this is non-critical
			}
		}
	} else {
		log.Printf("No counselor assigned for lead ID %d, skipping email", event.LeadID)
	}

	return nil
}
