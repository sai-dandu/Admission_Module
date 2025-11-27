package handlers

import (
	"admission-module/db"
	"admission-module/http/response"
	"admission-module/services"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// ApplicationActionHandler handles application accept/reject requests
func ApplicationActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		StudentID        int    `json:"student_id"`
		Status           string `json:"status"`
		SelectedCourseID *int   `json:"selected_course_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if req.Status != "ACCEPT" && req.Status != "REJECT" && req.Status != "ACCEPTED" && req.Status != "REJECTED" {
		response.ErrorResponse(w, http.StatusBadRequest, "Invalid status. Must be ACCEPT, REJECT, ACCEPTED, or REJECTED")
		return
	}

	// Normalize status values
	if req.Status == "ACCEPT" {
		req.Status = "ACCEPTED"
	} else if req.Status == "REJECT" {
		req.Status = "REJECTED"
	}

	if req.Status == "ACCEPTED" && req.SelectedCourseID == nil {
		response.ErrorResponse(w, http.StatusBadRequest, "Selected course ID is required for acceptance")
		return
	}

	// REQUIREMENT: Check if registration fee is PAID before allowing application status updates
	var regPaymentStatus string
	err := db.DB.QueryRow("SELECT status FROM registration_payment WHERE student_id = $1", req.StudentID).Scan(&regPaymentStatus)
	if err == sql.ErrNoRows {
		response.ErrorResponse(w, http.StatusBadRequest, "Registration payment record not found. Please complete registration fee payment first")
		return
	}
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "Error checking registration payment status")
		return
	}
	if regPaymentStatus != "PAID" {
		response.ErrorResponse(w, http.StatusBadRequest, "Application status cannot be updated. Registration payment status is "+regPaymentStatus+". Please complete registration fee payment first")
		return
	}

	appService := services.NewApplicationService()

	if req.Status == "ACCEPTED" {
		handleApplicationAcceptance(w, appService, req.StudentID, *req.SelectedCourseID)
	} else {
		handleApplicationRejection(w, appService, req.StudentID)
	}
}

func handleApplicationAcceptance(w http.ResponseWriter, appService *services.ApplicationService, studentID, courseID int) {
	result, err := appService.AcceptApplication(services.AcceptApplicationRequest{
		StudentID:        studentID,
		SelectedCourseID: courseID,
	})
	if err != nil {
		log.Printf("Error accepting application: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Send acceptance email asynchronously via Kafka
	go func() {
		if err := services.SendAcceptanceEmail(result.StudentName, result.StudentEmail, result.CourseName, result.CourseFee); err != nil {
			log.Printf("Warning: failed to queue acceptance email: %v", err)
		}
	}()

	response.SuccessResponse(w, http.StatusOK, "Application accepted successfully", map[string]interface{}{
		"student_id":      studentID,
		"student_name":    result.StudentName,
		"student_email":   result.StudentEmail,
		"selected_course": result.CourseName,
		"course_id":       result.CourseID,
		"course_fee":      result.CourseFee,
		"next_step":       "Please proceed with course fee payment",
		"payment_details": map[string]interface{}{
			"payment_type": "COURSE_FEE",
			"amount":       result.CourseFee,
			"currency":     "INR",
			"course_id":    result.CourseID,
		},
	})
}

func handleApplicationRejection(w http.ResponseWriter, appService *services.ApplicationService, studentID int) {
	result, err := appService.RejectApplication(services.RejectApplicationRequest{
		StudentID: studentID,
	})
	if err != nil {
		log.Printf("Error rejecting application: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Send rejection email asynchronously via Kafka
	go func() {
		if err := services.SendRejectionEmail(result.StudentName, result.StudentEmail); err != nil {
			log.Printf("Warning: failed to queue rejection email: %v", err)
		}
	}()

	response.SuccessResponse(w, http.StatusOK, "Application rejected successfully", map[string]interface{}{
		"student_id":    studentID,
		"student_name":  result.StudentName,
		"student_email": result.StudentEmail,
		"result":        "rejected",
		"notification":  "Rejection email has been sent to the student",
	})
}

func ApplicationAction(w http.ResponseWriter, r *http.Request) {
	ApplicationActionHandler(w, r)
}
