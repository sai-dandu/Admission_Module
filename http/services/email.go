package services

import (
	"errors"
	"os"
	"strconv"

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
		return errors.New("smtp credentials not configured (set SMTP_USER and SMTP_PASS)")
	}

	d := gomail.NewDialer(host, port, smtpUser, smtpPass)

	return d.DialAndSend(m)
}
