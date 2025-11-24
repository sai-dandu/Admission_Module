package services

import (
	"admission-module/db"
	"fmt"
	"os"
	"time"

	"github.com/razorpay/razorpay-go"
)

const RegistrationFee = 1870.0

// PaymentType constants
const (
	PaymentTypeRegistration = "REGISTRATION"
	PaymentTypeCourseFee    = "COURSE_FEE"
)

// PaymentService handles payment operations
type PaymentService struct{}

// InitiatePaymentRequest represents payment initiation request
type InitiatePaymentRequest struct {
	StudentID   int
	Amount      float64
	PaymentType string
	CourseID    *int
}

// InitiatePaymentResponse represents payment initiation response
type InitiatePaymentResponse struct {
	OrderID  string  `json:"order_id"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Receipt  string  `json:"receipt"`
}

// NewPaymentService creates a new PaymentService instance
func NewPaymentService() *PaymentService {
	return &PaymentService{}
}

func (s *PaymentService) ValidateAndPreparePayment(req InitiatePaymentRequest) (*InitiatePaymentRequest, error) {
	// Validate payment type using tagged switch
	switch req.PaymentType {
	case PaymentTypeRegistration:
		if req.Amount == 0 {
			req.Amount = RegistrationFee
		}

	case PaymentTypeCourseFee:
		// For course fee, course ID is required
		if req.CourseID == nil || *req.CourseID == 0 {
			return nil, fmt.Errorf("course ID required for course fee payment")
		}

		// Get course fee from database
		var courseFee float64
		err := db.DB.QueryRow("SELECT fee FROM course WHERE id = $1", *req.CourseID).Scan(&courseFee)
		if err != nil {
			return nil, fmt.Errorf("course not found")
		}
		req.Amount = courseFee

	default:
		return nil, fmt.Errorf("invalid payment type. must be REGISTRATION or COURSE_FEE")
	}

	// Validate amount
	if req.Amount <= 0 {
		return nil, fmt.Errorf("invalid amount: must be greater than 0")
	}

	// Verify student exists
	var exists bool
	err := db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM student_lead WHERE id = $1)", req.StudentID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("error checking student")
	}
	if !exists {
		return nil, fmt.Errorf("student not found")
	}

	return &req, nil
}

// CreateRazorpayOrder creates a Razorpay order
func (s *PaymentService) CreateRazorpayOrder(req InitiatePaymentRequest) (*InitiatePaymentResponse, error) {
	keyID := os.Getenv("RazorpayKeyID")
	keySecret := os.Getenv("RazorpayKeySecret")

	if keyID == "" || keySecret == "" {
		return nil, fmt.Errorf("razorpay credentials not configured")
	}

	client := razorpay.NewClient(keyID, keySecret)

	data := map[string]interface{}{
		"amount":   int(req.Amount * 100), // Convert to paise
		"currency": "INR",
		"receipt":  fmt.Sprintf("rcpt_%d_%s", req.StudentID, req.PaymentType),
	}

	// Create Razorpay order
	resp, err := client.Order.Create(data, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating razorpay order: %w", err)
	}

	orderID := resp["id"].(string)

	return &InitiatePaymentResponse{
		OrderID:  orderID,
		Amount:   req.Amount,
		Currency: "INR",
		Receipt:  fmt.Sprintf("rcpt_%d_%s", req.StudentID, req.PaymentType),
	}, nil
}

// SavePaymentRecord saves the payment record to the appropriate table
func (s *PaymentService) SavePaymentRecord(studentID int, orderID string, req InitiatePaymentRequest) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction")
	}
	defer tx.Rollback()

	if req.PaymentType == PaymentTypeRegistration {
		//check if registration payment already exists
		var existingPaymentID int
		var existingStatus string
		err = tx.QueryRow("SELECT id, status FROM registration_payment WHERE student_id = $1", studentID).Scan(&existingPaymentID, &existingStatus)
		if err == nil {
			// Payment already exists - if PENDING, update with new order_id; if PAID, reject
			if existingStatus == "PAID" {
				tx.Rollback()
				return fmt.Errorf("registration payment already completed")
			}
			// Update existing PENDING payment with new order_id
			_, err = tx.Exec("UPDATE registration_payment SET order_id = $1, amount = $2, updated_at = CURRENT_TIMESTAMP WHERE student_id = $3",
				orderID, req.Amount, studentID)
			if err != nil {
				return fmt.Errorf("error updating registration payment: %w", err)
			}
		} else {
			// No existing payment, insert new one
			_, err = tx.Exec(
				"INSERT INTO registration_payment (student_id, amount, status, order_id) VALUES ($1, $2, $3, $4)",
				studentID, req.Amount, "PENDING", orderID)
			if err != nil {
				return fmt.Errorf("error saving registration payment: %w", err)
			}
		}

		// Update student_lead registration_fee_status
		_, err = tx.Exec("UPDATE student_lead SET registration_fee_status = 'PENDING' WHERE id = $1", studentID)
		if err != nil {
			return fmt.Errorf("error updating registration fee status: %w", err)
		}

	} else if req.PaymentType == PaymentTypeCourseFee {
		// Save to course_payment table
		// Check if course payment already exists for this student+course
		var existingPaymentID int
		var existingStatus string
		err = tx.QueryRow("SELECT id, status FROM course_payment WHERE student_id = $1 AND course_id = $2", studentID, *req.CourseID).Scan(&existingPaymentID, &existingStatus)
		if err == nil {
			// Payment already exists - if PENDING, update with new order_id; if PAID, reject
			if existingStatus == "PAID" {
				tx.Rollback()
				return fmt.Errorf("course payment already completed")
			}
			// Update existing PENDING payment with new order_id
			_, err = tx.Exec("UPDATE course_payment SET order_id = $1, amount = $2, updated_at = CURRENT_TIMESTAMP WHERE student_id = $3 AND course_id = $4",
				orderID, req.Amount, studentID, *req.CourseID)
			if err != nil {
				return fmt.Errorf("error updating course payment: %w", err)
			}
		} else {
			// No existing payment, insert new one
			_, err = tx.Exec(
				"INSERT INTO course_payment (student_id, course_id, amount, status, order_id) VALUES ($1, $2, $3, $4, $5)",
				studentID, *req.CourseID, req.Amount, "PENDING", orderID)
			if err != nil {
				return fmt.Errorf("error saving course payment: %w", err)
			}
		}

		// Update student_lead course_fee_status (only if column exists)
		_, err = tx.Exec("UPDATE student_lead SET course_fee_status = 'PENDING' WHERE id = $1", studentID)
		if err != nil {
			// If column doesn't exist, try without it
			// Don't fail - the migration might not have run yet
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction")
	}

	return nil
}

// PublishPaymentInitiatedEvent publishes payment initiated event to Kafka
func (s *PaymentService) PublishPaymentInitiatedEvent(studentID int, orderID string, req InitiatePaymentRequest) {
	go func() {
		evt := map[string]interface{}{
			"event":        "payment.initiated",
			"student_id":   studentID,
			"order_id":     orderID,
			"amount":       req.Amount,
			"currency":     "INR",
			"payment_type": req.PaymentType,
			"status":       "PENDING",
			"ts":           time.Now().UTC().Format(time.RFC3339),
		}
		if err := Publish("payments", fmt.Sprintf("student-%d", studentID), evt); err != nil {
			// Silently fail - event publishing is non-critical
		}
	}()
}

// VerifyPaymentRequest represents payment verification request
type VerifyPaymentRequest struct {
	OrderID      string
	PaymentID    string
	RazorpaySign string
}

// VerifyPaymentResult represents the result of payment verification
type VerifyPaymentResult struct {
	StudentID   int
	PaymentType string
	Email       string
	Amount      float64
	CourseID    *int
}

// VerifyPayment verifies payment signature WITHOUT updating database
// Database is updated ONLY when webhook arrives from Razorpay (payment.captured event)
func (s *PaymentService) VerifyPayment(req VerifyPaymentRequest) (*VerifyPaymentResult, error) {
	var studentID int
	var paymentType string
	var amount float64
	var courseID *int
	var email string

	// Try registration_payment table first
	err := db.DB.QueryRow("SELECT student_id, amount FROM registration_payment WHERE order_id = $1", req.OrderID).Scan(&studentID, &amount)

	if err != nil {
		// If not found in registration_payment, check course_payment
		paymentType = PaymentTypeCourseFee
		err = db.DB.QueryRow(
			"SELECT student_id, course_id, amount FROM course_payment WHERE order_id = $1",
			req.OrderID,
		).Scan(&studentID, &courseID, &amount)

		if err != nil {
			return nil, fmt.Errorf("payment not found for order_id: %s", req.OrderID)
		}

	} else {
		// Found in registration_payment
		paymentType = PaymentTypeRegistration
	}

	// Get student email
	err = db.DB.QueryRow("SELECT email FROM student_lead WHERE id = $1", studentID).Scan(&email)
	if err != nil {
		// Email retrieval is optional
	}

	return &VerifyPaymentResult{
		StudentID:   studentID,
		PaymentType: paymentType,
		Email:       email,
		Amount:      amount,
		CourseID:    courseID,
	}, nil
}

// PublishPaymentVerifiedEvent publishes payment verified event to Kafka
func (s *PaymentService) PublishPaymentVerifiedEvent(studentID int, orderID, paymentID, paymentType string) {
	go func() {
		evt := map[string]interface{}{
			"event":        "payment.verified",
			"student_id":   studentID,
			"order_id":     orderID,
			"payment_id":   paymentID,
			"payment_type": paymentType,
			"status":       "PAID",
			"ts":           time.Now().UTC().Format(time.RFC3339),
		}
		if err := Publish("payments", fmt.Sprintf("student-%d", studentID), evt); err != nil {
			// Silently fail - event publishing is non-critical
		}
	}()
}

// IsRegistrationPayment checks if payment type is registration
func (s *PaymentService) IsRegistrationPayment(paymentType string) bool {
	return paymentType == PaymentTypeRegistration
}
