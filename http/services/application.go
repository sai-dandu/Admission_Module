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
