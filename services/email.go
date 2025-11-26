package services

import (
	"fmt"
	"log"
	"time"
)

// SendEmail publishes email event to Kafka for async processing
// Email will NOT be sent directly - instead it's queued via Kafka
// Kafka Consumer will handle the actual email sending
func SendEmail(to, subject, body string, attachment ...string) error {
	log.Printf("Publishing email event to Kafka. Recipient: %s, Subject: %s", to, subject)

	// Build email payload
	emailPayload := map[string]interface{}{
		"event":     "email.send",
		"recipient": to,
		"subject":   subject,
		"body":      body,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	// Add attachment if provided
	if len(attachment) > 0 {
		emailPayload["attachment"] = attachment[0]
	}

	// Publish to Kafka emails topic
	if err := Publish("emails", fmt.Sprintf("email-%s", to), emailPayload); err != nil {
		log.Printf("Failed to publish email event to Kafka: %v", err)
		return fmt.Errorf("failed to queue email: %w", err)
	}

	log.Printf("Email event queued to Kafka: %s", to)
	return nil
}

// SendAcceptanceEmail sends acceptance email via Kafka
func SendAcceptanceEmail(studentName, studentEmail, courseName string, courseFee float64) error {
	emailBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; border-radius: 5px; }
        .content { background-color: #f9f9f9; padding: 20px; margin-top: 20px; border-radius: 5px; }
        .course-info { background-color: #e8f5e9; padding: 15px; margin: 15px 0; border-left: 4px solid #4CAF50; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header"><h2>Congratulations!</h2></div>
        <div class="content">
            <p>Dear <strong>%s</strong>,</p>
            <p>We are pleased to inform you that your application has been <strong>ACCEPTED</strong>!</p>
            <div class="course-info">
                <p><strong>Selected Course:</strong> %s</p>
                <p><strong>Course Fee:</strong> ₹%.2f</p>
            </div>
            <p>To complete your admission, please proceed with the course fee payment.</p>
            <p>Best regards,<br/>University Admissions Team</p>
        </div>
    </div>
</body>
</html>
	`, studentName, courseName, courseFee)

	subject := fmt.Sprintf("Congratulations %s - Your Application is Accepted!", studentName)

	if err := SendEmail(studentEmail, subject, emailBody); err != nil {
		return err
	}

	return nil
}

// SendRejectionEmail sends rejection email via Kafka
func SendRejectionEmail(studentName, studentEmail string) error {
	emailBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background-color: #f44336; color: white; padding: 20px; text-align: center; border-radius: 5px; }
        .content { background-color: #f9f9f9; padding: 20px; margin-top: 20px; border-radius: 5px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header"><h2>Application Status</h2></div>
        <div class="content">
            <p>Dear <strong>%s</strong>,</p>
            <p>We regret to inform you that your application has been <strong>REJECTED</strong> at this time.</p>
            <p>We encourage you to apply again in future intake cycles.</p>
            <p>Best regards,<br/>University Admissions Team</p>
        </div>
    </div>
</body>
</html>
	`, studentName)

	subject := "Application Status - Rejection"

	if err := SendEmail(studentEmail, subject, emailBody); err != nil {
		return err
	}

	return nil
}
