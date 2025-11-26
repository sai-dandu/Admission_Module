package services

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"gopkg.in/gomail.v2"
)

// SendEmailDirect sends email directly via SMTP
// Called by Kafka consumer after receiving an email.send event
func SendEmailDirect(to, subject, body string, attachment ...string) error {
	log.Printf("🔄 Sending email via SMTP - Recipient: %s", to)

	m := gomail.NewMessage()

	from := os.Getenv("EMAIL_FROM")
	smtpUser := os.Getenv("SMTP_USER")
	if from == "" {
		from = smtpUser
	}
	if from == "" {
		log.Printf("❌ Email configuration error: sender not configured")
		return fmt.Errorf("email sender not configured (set EMAIL_FROM or SMTP_USER)")
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
		log.Printf("❌ Email configuration error: SMTP credentials not configured")
		return fmt.Errorf("smtp credentials not configured (set SMTP_USER and SMTP_PASS)")
	}

	d := gomail.NewDialer(host, port, smtpUser, smtpPass)

	err := d.DialAndSend(m)
	if err != nil {
		log.Printf("❌ Failed to send email to %s: %v", to, err)
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.Printf("✅ Email successfully sent to: %s", to)
	return nil
}
