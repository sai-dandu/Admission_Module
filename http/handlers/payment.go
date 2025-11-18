package handlers

import (
	"admission-module/db"
	resp "admission-module/http/response"
	"admission-module/http/services"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// InitiatePaymentHandler handles payment initiation requests
func InitiatePaymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		resp.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		StudentID   int     `json:"student_id"`
		Amount      float64 `json:"amount"`
		PaymentType string  `json:"payment_type"`
		CourseID    *int    `json:"course_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorResponse(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Default to registration payment if not specified
	if req.PaymentType == "" {
		req.PaymentType = services.PaymentTypeRegistration
	}

	paymentService := services.NewPaymentService()

	// Validate and prepare payment
	preparedReq, err := paymentService.ValidateAndPreparePayment(services.InitiatePaymentRequest{
		StudentID:   req.StudentID,
		Amount:      req.Amount,
		PaymentType: req.PaymentType,
		CourseID:    req.CourseID,
	})
	if err != nil {
		log.Printf("Payment validation error: %v", err)
		resp.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	// Create Razorpay order
	orderResp, err := paymentService.CreateRazorpayOrder(*preparedReq)
	if err != nil {
		log.Printf("Razorpay order creation error: %v", err)
		resp.ErrorResponse(w, http.StatusInternalServerError, "Error creating payment order")
		return
	}

	// Save payment record
	if err := paymentService.SavePaymentRecord(req.StudentID, orderResp.OrderID, *preparedReq); err != nil {
		log.Printf("[PAYMENT] Payment record save error - StudentID: %d, OrderID: %s, Error: %v", req.StudentID, orderResp.OrderID, err)
		resp.ErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error saving payment record: %v", err))
		return
	}

	// Publish event asynchronously
	paymentService.PublishPaymentInitiatedEvent(req.StudentID, orderResp.OrderID, *preparedReq)

	log.Printf("[PAYMENT] Sending response to client - OrderID: '%s', Amount: %.2f, Currency: %s",
		orderResp.OrderID, orderResp.Amount, orderResp.Currency)

	resp.SuccessResponse(w, http.StatusOK, "Payment order created", orderResp)
}

// VerifyPaymentHandler handles payment verification requests
func VerifyPaymentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		resp.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		OrderID      string `json:"order_id"`
		PaymentID    string `json:"payment_id"`
		RazorpaySign string `json:"razorpay_signature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[PAYMENT] Request decode error: %v", err)
		resp.ErrorResponse(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Validate required fields
	log.Printf("[PAYMENT] Verify request received - OrderID: '%s', PaymentID: '%s', Signature: '%s'",
		req.OrderID, req.PaymentID, req.RazorpaySign)

	if req.OrderID == "" {
		log.Printf("[PAYMENT] ERROR: OrderID is empty in verify request")
		resp.ErrorResponse(w, http.StatusBadRequest, "order_id is required and cannot be empty")
		return
	}
	if req.PaymentID == "" {
		log.Printf("[PAYMENT] ERROR: PaymentID is empty in verify request")
		resp.ErrorResponse(w, http.StatusBadRequest, "payment_id is required and cannot be empty")
		return
	}
	if req.RazorpaySign == "" {
		log.Printf("[PAYMENT] ERROR: RazorpaySign is empty in verify request")
		resp.ErrorResponse(w, http.StatusBadRequest, "razorpay_signature is required and cannot be empty")
		return
	}

	paymentService := services.NewPaymentService()

	// Verify payment
	result, err := paymentService.VerifyPayment(services.VerifyPaymentRequest{
		OrderID:      req.OrderID,
		PaymentID:    req.PaymentID,
		RazorpaySign: req.RazorpaySign,
	})
	if err != nil {
		log.Printf("Payment verification error: %v", err)
		resp.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Publish payment verified event
	paymentService.PublishPaymentVerifiedEvent(result.StudentID, req.OrderID, req.PaymentID, result.PaymentType)

	// If registration payment, schedule interview asynchronously
	if paymentService.IsRegistrationPayment(result.PaymentType) {
		go func() {
			scheduleInterviewAfterPayment(result.StudentID, result.Email)
		}()
	}

	// Return success response
	resp.SuccessResponse(w, http.StatusOK, "Payment verified successfully", map[string]interface{}{
		"status":     "PAID",
		"student_id": result.StudentID,
		"order_id":   req.OrderID,
	})
}

// scheduleInterviewAfterPayment schedules interview for the student after successful registration payment
func scheduleInterviewAfterPayment(studentID int, email string) {
	// Schedule meet
	meetLink, err := services.ScheduleMeet(email)
	if err != nil {
		log.Printf("Error scheduling meet for student %d: %v", studentID, err)
		return
	}

	// Get database instance and schedule interview
	interviewTime := time.Now().Add(24 * time.Hour)

	// Update DB with meet link and interview scheduled time
	_, err = db.DB.Exec(
		"UPDATE student_lead SET meet_link = $1, application_status = 'INTERVIEW_SCHEDULED', interview_scheduled_at = $2 WHERE id = $3",
		meetLink, interviewTime, studentID)
	if err != nil {
		log.Printf("Error updating lead with meet link: %v", err)
		return
	}

	log.Printf("Interview scheduled successfully for student %d at %s", studentID, interviewTime.Format(time.RFC3339))

	// Publish interview scheduled event
	evt := map[string]interface{}{
		"event":                  "interview.scheduled",
		"student_id":             studentID,
		"email":                  email,
		"meet_link":              meetLink,
		"interview_scheduled_at": interviewTime.Format(time.RFC3339),
		"status":                 "scheduled",
		"ts":                     time.Now().UTC().Format(time.RFC3339),
	}
	if err := services.Publish("meetings", fmt.Sprintf("student-%d", studentID), evt); err != nil {
		log.Printf("Warning: failed to publish interview.scheduled event: %v", err)
	}
}

// Backward compatibility wrappers
func InitiatePayment(w http.ResponseWriter, r *http.Request) {
	InitiatePaymentHandler(w, r)
}

func VerifyPayment(w http.ResponseWriter, r *http.Request) {
	VerifyPaymentHandler(w, r)
}
