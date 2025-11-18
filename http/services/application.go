package services

import (
	"admission-module/db"
	"fmt"
	"log"
	"time"
)

// ApplicationService handles all application review operations
type ApplicationService struct{}

// AcceptApplicationRequest represents the request for accepting an application
type AcceptApplicationRequest struct {
	StudentID        int
	SelectedCourseID int
}

// AcceptApplicationResult contains the result of accepting an application
type AcceptApplicationResult struct {
	StudentName  string
	StudentEmail string
	CourseName   string
	CourseFee    float64
	CourseID     int
}

// RejectApplicationRequest represents the request for rejecting an application
type RejectApplicationRequest struct {
	StudentID int
}

// RejectApplicationResult contains the result of rejecting an application
type RejectApplicationResult struct {
	StudentName  string
	StudentEmail string
}

// NewApplicationService creates a new ApplicationService instance
func NewApplicationService() *ApplicationService {
	return &ApplicationService{}
}

// AcceptApplication accepts an application and returns course details
func (s *ApplicationService) AcceptApplication(req AcceptApplicationRequest) (*AcceptApplicationResult, error) {
	// Get student details
	var name, email string
	err := db.DB.QueryRow("SELECT name, email FROM student_lead WHERE id = $1", req.StudentID).Scan(&name, &email)
	if err != nil {
		return nil, fmt.Errorf("student not found")
	}

	// Get course details
	var courseName string
	var courseFee float64
	err = db.DB.QueryRow("SELECT name, fee FROM course WHERE id = $1", req.SelectedCourseID).Scan(&courseName, &courseFee)
	if err != nil {
		return nil, fmt.Errorf("course not found")
	}

	// Update application status
	_, err = db.DB.Exec(
		"UPDATE student_lead SET application_status = $1, selected_course_id = $2 WHERE id = $3",
		"ACCEPTED", req.SelectedCourseID, req.StudentID)
	if err != nil {
		return nil, fmt.Errorf("error updating lead status")
	}

	log.Printf("Application accepted for student: %s (ID: %d) - Course: %s", name, req.StudentID, courseName)

	return &AcceptApplicationResult{
		StudentName:  name,
		StudentEmail: email,
		CourseName:   courseName,
		CourseFee:    courseFee,
		CourseID:     req.SelectedCourseID,
	}, nil
}

// RejectApplication rejects an application
func (s *ApplicationService) RejectApplication(req RejectApplicationRequest) (*RejectApplicationResult, error) {
	// Get student details
	var name, email string
	err := db.DB.QueryRow("SELECT name, email FROM student_lead WHERE id = $1", req.StudentID).Scan(&name, &email)
	if err != nil {
		return nil, fmt.Errorf("student not found")
	}

	// Update application status
	_, err = db.DB.Exec("UPDATE student_lead SET application_status = $1 WHERE id = $2", "REJECTED", req.StudentID)
	if err != nil {
		return nil, fmt.Errorf("error updating lead status")
	}

	log.Printf("Application rejected for student: %s (ID: %d)", name, req.StudentID)

	return &RejectApplicationResult{
		StudentName:  name,
		StudentEmail: email,
	}, nil
}

// SendAcceptanceEmail sends congratulations email with course details
func (s *ApplicationService) SendAcceptanceEmail(studentName, studentEmail, courseName string, courseFee float64) error {
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
	return SendEmail(studentEmail, subject, emailBody)
}

// SendRejectionEmail sends rejection notification email
func (s *ApplicationService) SendRejectionEmail(studentName, studentEmail string) error {
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
	return SendEmail(studentEmail, subject, emailBody)
}

// PublishApplicationEvent publishes application events to Kafka
func PublishApplicationEvent(eventType string, studentID int, email, course string, status string) {
	go func() {
		evt := map[string]interface{}{
			"event":      "application." + eventType,
			"student_id": studentID,
			"email":      email,
			"course":     course,
			"status":     status,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		}
		if err := Publish("applications", fmt.Sprintf("student-%d", studentID), evt); err != nil {
			log.Printf("Warning: failed to publish application event: %v", err)
		}
	}()
}
