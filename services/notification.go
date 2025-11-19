package services

import (
	"admission-module/db"
	"admission-module/models"
	"context"
	"fmt"
	"log"
)

// SendWelcomeEmailWithCounselorInfo sends welcome email to student with counselor details
// This function retrieves counselor information and sends both student welcome and counselor notification emails
func SendWelcomeEmailWithCounselorInfo(ctx context.Context, lead *models.Lead) error {
	if lead.CounsellorID == nil {
		log.Printf("Warning: Lead %d has no assigned counselor, skipping email", lead.ID)
		return nil
	}

	// Get counselor information
	var counselorName, counselorEmail, counselorPhone string
	query := "SELECT name, email, phone FROM counselor WHERE id = $1"
	err := db.DB.QueryRowContext(ctx, query, *lead.CounsellorID).Scan(&counselorName, &counselorEmail, &counselorPhone)
	if err != nil {
		log.Printf("Error fetching counselor details: %v", err)
		return fmt.Errorf("error fetching counselor details: %w", err)
	}

	// Send student welcome email
	if err := SendCounselorAssignmentEmail(lead.Name, lead.Email, counselorName, counselorEmail, counselorPhone); err != nil {
		log.Printf("Error sending welcome email to student: %v", err)
		return err
	}

	// Send counselor notification email
	if err := SendCounselorAssignmentNotificationEmail(counselorName, counselorEmail, lead.Name, lead.Phone, lead.Email, lead.LeadSource); err != nil {
		log.Printf("Error sending notification email to counselor: %v", err)
		// Don't return error - this is non-critical
	}

	return nil
}

// SendCounselorAssignmentEmail sends a welcome email to the student with counselor information
func SendCounselorAssignmentEmail(studentName, studentEmail, counselorName, counselorEmail, counselorPhone string) error {
	if studentEmail == "" {
		return fmt.Errorf("student email is required")
	}

	log.Printf("Sending welcome email to student: %s (%s) with counselor: %s", studentName, studentEmail, counselorName)

	emailBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f9f9f9;
        }
        .header {
            background-color: #4CAF50;
            color: white;
            padding: 20px;
            text-align: center;
            border-radius: 5px;
        }
        .content {
            background-color: white;
            padding: 20px;
            margin-top: 20px;
            border-radius: 5px;
        }
        .counselor-info {
            background-color: #e8f5e9;
            padding: 15px;
            margin: 15px 0;
            border-left: 4px solid #4CAF50;
            border-radius: 3px;
        }
        .info-item {
            margin: 10px 0;
            font-size: 14px;
        }
        .label {
            font-weight: bold;
            color: #2196F3;
        }
        .footer {
            text-align: center;
            margin-top: 20px;
            padding-top: 20px;
            border-top: 1px solid #ddd;
            font-size: 12px;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>Welcome to Our University</h2>
        </div>
        
        <div class="content">
            <p>Dear <strong>%s</strong>,</p>
            
            <p>Welcome to our admission process! We are excited to have you join us.</p>
            
            <h3>Your Assigned Counselor</h3>
            <p>Your application has been assigned to a dedicated counselor who will guide you through every step of the admission journey.</p>
            
            <div class="counselor-info">
                <div class="info-item">
                    <span class="label">Counselor Name:</span> %s
                </div>
                <div class="info-item">
                    <span class="label">Email:</span> <a href="mailto:%s">%s</a>
                </div>
                <div class="info-item">
                    <span class="label">Phone:</span> <a href="tel:%s">%s</a>
                </div>
            </div>
            
            <p>Your counselor will contact you shortly to discuss your admission requirements and answer any questions you may have.</p>
            
            <h3>Next Steps</h3>
            <ul>
                <li>Complete your registration by paying the registration fee of ₹1,870</li>
                <li>Wait for your interview scheduling confirmation</li>
                <li>Prepare for your interview</li>
                <li>Upon acceptance, complete the course fee payment</li>
            </ul>
            
            <p>If you have any questions or need assistance, please don't hesitate to contact your counselor directly.</p>
            
            <p>Best regards,<br/>
            <strong>University Admissions Team</strong></p>
            
            <div class="footer">
                <p>This is an automated email. Please do not reply to this address.</p>
                <p>&copy; 2025 Our University. All rights reserved.</p>
            </div>
        </div>
    </div>
</body>
</html>
	`, studentName, counselorName, counselorEmail, counselorEmail, counselorPhone, counselorPhone)

	subject := fmt.Sprintf("Welcome %s - Your Counselor Assignment", studentName)
	err := SendEmail(studentEmail, subject, emailBody)
	if err != nil {
		log.Printf("Error sending welcome email to %s: %v", studentEmail, err)
		return err
	}
	log.Printf("Successfully sent welcome email to %s", studentEmail)
	return nil
}

// SendCounselorAssignmentNotificationEmail sends a notification to the counselor about new assignment
func SendCounselorAssignmentNotificationEmail(counselorName, counselorEmail, studentName, studentPhone, studentEmail, leadSource string) error {
	if counselorEmail == "" {
		return fmt.Errorf("counselor email is required")
	}

	log.Printf("Sending counselor notification email to: %s (%s) for student: %s", counselorName, counselorEmail, studentName)

	emailBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body {
            font-family: Arial, sans-serif;
            line-height: 1.6;
            color: #333;
        }
        .container {
            max-width: 600px;
            margin: 0 auto;
            padding: 20px;
            background-color: #f9f9f9;
        }
        .header {
            background-color: #2196F3;
            color: white;
            padding: 20px;
            text-align: center;
            border-radius: 5px;
        }
        .content {
            background-color: white;
            padding: 20px;
            margin-top: 20px;
            border-radius: 5px;
        }
        .student-info {
            background-color: #e3f2fd;
            padding: 15px;
            margin: 15px 0;
            border-left: 4px solid #2196F3;
            border-radius: 3px;
        }
        .info-item {
            margin: 8px 0;
            font-size: 14px;
        }
        .label {
            font-weight: bold;
            color: #1976D2;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h2>New Lead Assignment</h2>
        </div>
        
        <div class="content">
            <p>Dear <strong>%s</strong>,</p>
            
            <p>A new lead has been assigned to you in the admission system.</p>
            
            <div class="student-info">
                <div class="info-item">
                    <span class="label">Student Name:</span> %s
                </div>
                <div class="info-item">
                    <span class="label">Email:</span> <a href="mailto:%s">%s</a>
                </div>
                <div class="info-item">
                    <span class="label">Phone:</span> <a href="tel:%s">%s</a>
                </div>
                <div class="info-item">
                    <span class="label">Lead Source:</span> %s
                </div>
            </div>
            
            <p>Please reach out to the student at your earliest convenience to welcome them and guide them through the admission process.</p>
            
            <p>Best regards,<br/>
            <strong>Admission System</strong></p>
        </div>
    </div>
</body>
</html>
	`, counselorName, studentName, studentEmail, studentEmail, studentPhone, studentPhone, leadSource)

	subject := fmt.Sprintf("New Lead Assignment - %s", studentName)
	err := SendEmail(counselorEmail, subject, emailBody)
	if err != nil {
		log.Printf("Error sending counselor notification email to %s: %v", counselorEmail, err)
		return err
	}
	log.Printf("Successfully sent counselor notification email to %s", counselorEmail)
	return nil
}
