package services

import (
	"admission-module/db"
	"fmt"
	"log"
	"time"
)

// ScheduleMeet creates a meeting invite for the given email and stores meet_link in database.
// Instead of using Google Calendar API, it generates a simple meeting link and sends an email with the details.
func ScheduleMeet(studentID int, email string) (string, error) {
	// Generate a unique meeting ID using timestamp
	meetID := fmt.Sprintf("%d", time.Now().Unix())

	meetLink := fmt.Sprintf("https://meet.google.com/%s", meetID)

	// Schedule meeting for 1 hour from now
	meetTime := time.Now().Add(time.Hour)
	endTime := meetTime.Add(time.Hour)

	emailBody := fmt.Sprintf(`
        <h2>Meeting Scheduled</h2>
		<p>Your interview meeting with Sai University has been scheduled.<p>
        <p><strong>Date:</strong> %s</p>
        <p><strong>Time:</strong> %s - %s</p>
        <p><strong>Meeting Link:</strong> <a href="%s">%s</a></p>
        <p>Click the link above to join the meeting at the scheduled time.</p>
    `,
		meetTime.Format("Monday, January 2, 2006"),
		meetTime.Format("3:04 PM"),
		endTime.Format("3:04 PM"),
		meetLink,
		meetLink,
	)

	// Send the meeting invite via email
	err := SendEmail(
		email,
		fmt.Sprintf("Meeting Scheduled for %s", meetTime.Format("Jan 2, 2006 3:04 PM")),
		emailBody,
	)
	if err != nil {
		return "", fmt.Errorf("failed to send meeting invite: %w", err)
	}

	// Store meet_link in student_lead table
	log.Printf("Storing meet_link in database for student ID: %d", studentID)
	_, err = db.DB.Exec(
		"UPDATE student_lead SET meet_link = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		meetLink, studentID)
	if err != nil {
		log.Printf("Warning: failed to store meet_link in database: %v", err)
	} else {
		log.Printf("✅ meet_link stored in database: %s", meetLink)
	}

	return meetLink, nil
}
