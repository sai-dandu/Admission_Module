package handlers

import (
	resp "admission-module/http/response"
	"admission-module/services"
	"encoding/json"
	"net/http"
)

// InitiatePaymentHandler handles payment initiation requests
// This handler supports both registration and course fee payments
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

	// Parse request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorResponse(w, http.StatusBadRequest, "Invalid request format: "+err.Error())
		return
	}

	// Validate student ID
	if req.StudentID <= 0 {
		resp.ErrorResponse(w, http.StatusBadRequest, "Invalid student ID - must be greater than 0")
		return
	}

	// Default to registration payment if not specified
	if req.PaymentType == "" {
		req.PaymentType = services.PaymentTypeRegistration
	}

	// Validate payment type
	if req.PaymentType != services.PaymentTypeRegistration && req.PaymentType != services.PaymentTypeCourseFee {
		resp.ErrorResponse(w, http.StatusBadRequest, "Invalid payment type - must be REGISTRATION or COURSE_FEE")
		return
	}

	paymentService := services.NewPaymentService()

	// Check payment eligibility
	canPay, reason, err := paymentService.CheckPaymentEligibility(req.StudentID, req.PaymentType, req.CourseID)
	if err != nil {
		resp.ErrorResponse(w, http.StatusBadRequest, reason)
		return
	}
	if !canPay {
		resp.ErrorResponse(w, http.StatusBadRequest, reason)
		return
	}

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
		resp.ErrorResponse(w, http.StatusInternalServerError, "Error creating payment order: "+err.Error())
		return
	}

	// Save payment record
	if err := paymentService.SavePaymentRecord(req.StudentID, orderResp.OrderID, *preparedReq); err != nil {
		// Determine if this is a client error or server error
		if err.Error() == "registration payment already completed - student has already paid registration fee" ||
			err.Error() == "course payment already completed - student has already paid fee for course" {
			resp.ErrorResponse(w, http.StatusBadRequest, err.Error())
		} else {
			resp.ErrorResponse(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Publish event asynchronously
	paymentService.PublishPaymentInitiatedEvent(req.StudentID, orderResp.OrderID, *preparedReq)

	// Return success response with order details
	resp.SuccessResponse(w, http.StatusOK, "Payment order created successfully", map[string]interface{}{
		"order_id":     orderResp.OrderID,
		"amount":       orderResp.Amount,
		"currency":     orderResp.Currency,
		"receipt":      orderResp.Receipt,
		"payment_type": req.PaymentType,
		"student_id":   req.StudentID,
		"message":      "Please complete the payment using Razorpay",
	})
}

// VerifyPaymentHandler handles payment verification requests
// This is a client-side verification endpoint that checks signature
// The actual database update happens via Razorpay webhook
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

	// Parse request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		resp.ErrorResponse(w, http.StatusBadRequest, "Invalid request format: "+err.Error())
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
	_, err := paymentService.VerifyPayment(services.VerifyPaymentRequest{
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
	// 1. Verify signature again (server-side verification)
	// 2. Update database status to PAID
	// 3. Publish payment.verified event to Kafka
	// 4. Schedule interview (if registration payment)

	// Return success response (status still PENDING until webhook confirms)
	resp.SuccessResponse(w, http.StatusOK, "Payment verified successfully", map[string]interface{}{
		"status": "success",
	})
}

// GetPaymentStatusHandler returns the current payment status for an order
func GetPaymentStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		resp.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	orderID := r.URL.Query().Get("order_id")
	if orderID == "" {
		resp.ErrorResponse(w, http.StatusBadRequest, "order_id query parameter is required")
		return
	}

	paymentService := services.NewPaymentService()

	status, paymentType, studentID, err := paymentService.GetPaymentStatus(orderID)
	if err != nil {
		resp.ErrorResponse(w, http.StatusNotFound, "Payment not found for order_id: "+orderID)
		return
	}

	resp.SuccessResponse(w, http.StatusOK, "Payment status retrieved successfully", map[string]interface{}{
		"order_id":     orderID,
		"status":       status,
		"payment_type": paymentType,
		"student_id":   studentID,
	})
}

// Backward compatibility wrappers
func InitiatePayment(w http.ResponseWriter, r *http.Request) {
	InitiatePaymentHandler(w, r)
}

func VerifyPayment(w http.ResponseWriter, r *http.Request) {
	VerifyPaymentHandler(w, r)
}

func GetPaymentStatus(w http.ResponseWriter, r *http.Request) {
	GetPaymentStatusHandler(w, r)
}
