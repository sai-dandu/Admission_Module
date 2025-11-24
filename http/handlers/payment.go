package handlers

import (
	resp "admission-module/http/response"
	"admission-module/services"
	"encoding/json"
	"log"
	"net/http"
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
		// Check for client errors (already completed payments)
		if err.Error() == "registration payment already completed" || err.Error() == "course payment already completed" {
			resp.ErrorResponse(w, http.StatusBadRequest, err.Error())
		} else {
			resp.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Publish event asynchronously
	paymentService.PublishPaymentInitiatedEvent(req.StudentID, orderResp.OrderID, *preparedReq)

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
		resp.ErrorResponse(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	// Validate required fields
	if req.OrderID == "" {
		resp.ErrorResponse(w, http.StatusBadRequest, "order_id is required and cannot be empty")
		return
	}
	if req.PaymentID == "" {
		resp.ErrorResponse(w, http.StatusBadRequest, "payment_id is required and cannot be empty")
		return
	}
	if req.RazorpaySign == "" {
		resp.ErrorResponse(w, http.StatusBadRequest, "razorpay_signature is required and cannot be empty")
		return
	}

	paymentService := services.NewPaymentService()

	// Verify payment signature (this is client-side verification only)
	// The actual database update will happen when the webhook arrives from Razorpay
	result, err := paymentService.VerifyPayment(services.VerifyPaymentRequest{
		OrderID:      req.OrderID,
		PaymentID:    req.PaymentID,
		RazorpaySign: req.RazorpaySign,
	})
	if err != nil {
		resp.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	// NOTE: DO NOT update database here, DO NOT publish event here
	// The webhook (payment.captured) from Razorpay will:
	// 1. Verify signature again
	// 2. Update database status to PAID
	// 3. Publish payment.verified event to Kafka
	// 4. Schedule interview (if registration payment)

	// Return success response (status still PENDING until webhook confirms)
	resp.SuccessResponse(w, http.StatusOK, "Payment signature verified. Waiting for Razorpay webhook confirmation.", map[string]interface{}{
		"status":     "PENDING",
		"student_id": result.StudentID,
		"order_id":   req.OrderID,
		"message":    "Database will be updated when Razorpay webhook arrives",
	})
}

// Backward compatibility wrappers
func InitiatePayment(w http.ResponseWriter, r *http.Request) {
	InitiatePaymentHandler(w, r)
}

func VerifyPayment(w http.ResponseWriter, r *http.Request) {
	VerifyPaymentHandler(w, r)
}
