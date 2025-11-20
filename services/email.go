package services

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gopkg.in/gomail.v2"
)

// data from env
func SendEmail(to, subject, body string, attachment ...string) error {

	m := gomail.NewMessage()

	from := os.Getenv("EMAIL_FROM")
	smtpUser := os.Getenv("SMTP_USER")
	if from == "" {
		from = smtpUser
	}
	if from == "" {
		log.Printf("Email configuration error: sender not configured")
		return errors.New("email sender not configured (set EMAIL_FROM or SMTP_USER)")
	}

	m.SetHeader("From", from)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", body)

	if len(attachment) > 0 {
		m.Attach(attachment[0])
	}

	host := os.Getenv("SMTP_HOST")
	if host == "" {
		host = "smtp.gmail.com"
	}

	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	smtpPass := os.Getenv("SMTP_PASS")
	if smtpUser == "" || smtpPass == "" {
		log.Printf("Email configuration error: SMTP credentials not configured")
		return errors.New("smtp credentials not configured (set SMTP_USER and SMTP_PASS)")
	}

	d := gomail.NewDialer(host, port, smtpUser, smtpPass)

	err := d.DialAndSend(m)
	if err != nil {
		log.Printf("Failed to send email to %s: %v", to, err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("Email successfully sent to: %s", to)
	return nil
}

// SendAcceptanceEmail sends congratulations email with course details and publishes to Kafka
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
	return SendEmailWithKafkaEvent(studentEmail, subject, emailBody, "acceptance", map[string]interface{}{
		"student_name": studentName,
		"course_name":  courseName,
		"course_fee":   courseFee,
	})
}

// SendRejectionEmail sends rejection notification email and publishes to Kafka
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
	return SendEmailWithKafkaEvent(studentEmail, subject, emailBody, "rejection", map[string]interface{}{
		"student_name": studentName,
	})
}

// PublishEmailSentEvent publishes email sent event to Kafka
func PublishEmailSentEvent(emailType, recipient, subject string, metadata map[string]interface{}) {
	go func() {
		evt := map[string]interface{}{
			"event":     "email." + emailType,
			"recipient": recipient,
			"subject":   subject,
			"metadata":  metadata,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
		if err := Publish("emails", fmt.Sprintf("email-%s", recipient), evt); err != nil {
			log.Printf("Warning: failed to publish email.%s event: %v", emailType, err)
		}
	}()
}

// SendEmailWithKafkaEvent sends email and publishes event to Kafka
func SendEmailWithKafkaEvent(to, subject, body string, emailType string, metadata map[string]interface{}) error {
	// Send email
	if err := SendEmail(to, subject, body); err != nil {
		return err
	}

	// Publish email.sent event to Kafka (async, best-effort)
	PublishEmailSentEvent(emailType, to, subject, metadata)

	return nil
}
