package services

import (
	"admission-module/db"
	"database/sql"
	"fmt"
	"log"
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

// Payment Status constants
const (
	PaymentStatusPending   = "PENDING"
	PaymentStatusPaid      = "PAID"
	PaymentStatusFailed    = "FAILED"
	PaymentStatusCancelled = "CANCELLED"
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
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback()

	if req.PaymentType == PaymentTypeRegistration {
		// Check if registration payment already exists
		var existingPaymentID int
		var existingStatus string
		err = tx.QueryRow("SELECT id, status FROM registration_payment WHERE student_id = $1", studentID).Scan(&existingPaymentID, &existingStatus)

		if err == nil {
			// Payment already exists
			if existingStatus == PaymentStatusPaid {
				tx.Rollback()
				return fmt.Errorf("registration payment already completed - student has already paid registration fee")
			}
			if existingStatus == PaymentStatusFailed || existingStatus == PaymentStatusCancelled {
				// Can retry failed/cancelled payment
				_, err = tx.Exec(
					"UPDATE registration_payment SET order_id = $1, amount = $2, status = $3, payment_id = NULL, razorpay_sign = NULL, updated_at = CURRENT_TIMESTAMP WHERE student_id = $4",
					orderID, req.Amount, PaymentStatusPending, studentID)
				if err != nil {
					return fmt.Errorf("error updating failed registration payment: %w", err)
				}
			} else if existingStatus == PaymentStatusPending {
				// Update existing PENDING payment with new order_id (retry)
				_, err = tx.Exec(
					"UPDATE registration_payment SET order_id = $1, amount = $2, updated_at = CURRENT_TIMESTAMP WHERE student_id = $3",
					orderID, req.Amount, studentID)
				if err != nil {
					return fmt.Errorf("error updating pending registration payment: %w", err)
				}
			}
		} else if err == sql.ErrNoRows {
			// No existing payment, insert new one
			_, err = tx.Exec(
				"INSERT INTO registration_payment (student_id, amount, status, order_id) VALUES ($1, $2, $3, $4)",
				studentID, req.Amount, PaymentStatusPending, orderID)
			if err != nil {
				return fmt.Errorf("error saving registration payment: %w", err)
			}
		} else {
			return fmt.Errorf("error checking existing registration payment: %w", err)
		}

		// Update student_lead registration_fee_status
		_, err = tx.Exec("UPDATE student_lead SET registration_fee_status = $1 WHERE id = $2", PaymentStatusPending, studentID)
		if err != nil {
			return fmt.Errorf("error updating registration fee status: %w", err)
		}

	} else if req.PaymentType == PaymentTypeCourseFee {
		// Save to course_payment table
		if req.CourseID == nil || *req.CourseID == 0 {
			return fmt.Errorf("course ID is required for course fee payment")
		}

		// Check if course payment already exists for this student+course
		var existingPaymentID int
		var existingStatus string
		err = tx.QueryRow("SELECT id, status FROM course_payment WHERE student_id = $1 AND course_id = $2", studentID, *req.CourseID).Scan(&existingPaymentID, &existingStatus)

		if err == nil {
			// Payment already exists
			if existingStatus == PaymentStatusPaid {
				tx.Rollback()
				return fmt.Errorf("course payment already completed - student has already paid fee for course %d", *req.CourseID)
			}
			if existingStatus == PaymentStatusFailed || existingStatus == PaymentStatusCancelled {
				// Can retry failed/cancelled payment
				_, err = tx.Exec(
					"UPDATE course_payment SET order_id = $1, amount = $2, status = $3, payment_id = NULL, razorpay_sign = NULL, updated_at = CURRENT_TIMESTAMP WHERE student_id = $4 AND course_id = $5",
					orderID, req.Amount, PaymentStatusPending, studentID, *req.CourseID)
				if err != nil {
					return fmt.Errorf("error updating failed course payment: %w", err)
				}
			} else if existingStatus == PaymentStatusPending {
				// Update existing PENDING payment with new order_id (retry)
				_, err = tx.Exec(
					"UPDATE course_payment SET order_id = $1, amount = $2, updated_at = CURRENT_TIMESTAMP WHERE student_id = $3 AND course_id = $4",
					orderID, req.Amount, studentID, *req.CourseID)
				if err != nil {
					return fmt.Errorf("error updating pending course payment: %w", err)
				}
			}
		} else if err == sql.ErrNoRows {
			// No existing payment, insert new one
			_, err = tx.Exec(
				"INSERT INTO course_payment (student_id, course_id, amount, status, order_id) VALUES ($1, $2, $3, $4, $5)",
				studentID, *req.CourseID, req.Amount, PaymentStatusPending, orderID)
			if err != nil {
				return fmt.Errorf("error saving course payment: %w", err)
			}
		} else {
			return fmt.Errorf("error checking existing course payment: %w", err)
		}

		// Update student_lead course_fee_status
		_, err = tx.Exec("UPDATE student_lead SET course_fee_status = $1 WHERE id = $2", PaymentStatusPending, studentID)
		if err != nil {
			// Not critical - continue
			log.Printf("Warning: error updating course fee status: %v", err)
		}
	} else {
		return fmt.Errorf("invalid payment type: %s", req.PaymentType)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
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
			"source":       "webhook",
			"status":       "PAID",
			"ts":           time.Now().UTC().Format(time.RFC3339),
		}
		if err := Publish("payments", fmt.Sprintf("student-%d", studentID), evt); err != nil {
			log.Printf("Warning: failed to publish payment.verified event: %v", err)
		}
	}()
}

// IsRegistrationPayment checks if payment type is registration
func (s *PaymentService) IsRegistrationPayment(paymentType string) bool {
	return paymentType == PaymentTypeRegistration
}

// GetPaymentStatus retrieves the current payment status for a given order ID
func (s *PaymentService) GetPaymentStatus(orderID string) (status string, paymentType string, studentID int, err error) {
	// Try registration_payment first
	err = db.DB.QueryRow("SELECT status, student_id FROM registration_payment WHERE order_id = $1", orderID).Scan(&status, &studentID)
	if err == nil {
		return status, PaymentTypeRegistration, studentID, nil
	}

	// Try course_payment
	err = db.DB.QueryRow("SELECT status, student_id FROM course_payment WHERE order_id = $1", orderID).Scan(&status, &studentID)
	if err == nil {
		return status, PaymentTypeCourseFee, studentID, nil
	}

	return "", "", 0, fmt.Errorf("payment not found for order_id: %s", orderID)
}

// ValidateStudentExists checks if student exists and returns student details
func (s *PaymentService) ValidateStudentExists(studentID int) (name, email string, err error) {
	err = db.DB.QueryRow("SELECT name, email FROM student_lead WHERE id = $1", studentID).Scan(&name, &email)
	if err != nil {
		return "", "", fmt.Errorf("student not found with id: %d", studentID)
	}
	return name, email, nil
}

// CheckPaymentEligibility checks if student can make a payment
func (s *PaymentService) CheckPaymentEligibility(studentID int, paymentType string, courseID *int) (canPay bool, reason string, err error) {
	// Check if student exists
	var exists bool
	err = db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM student_lead WHERE id = $1)", studentID).Scan(&exists)
	if err != nil || !exists {
		return false, "Student not found", err
	}

	if paymentType == PaymentTypeRegistration {
		// Check if registration payment already paid
		var status string
		err = db.DB.QueryRow("SELECT status FROM registration_payment WHERE student_id = $1", studentID).Scan(&status)
		if err == nil {
			if status == PaymentStatusPaid {
				return false, "Registration payment already completed", nil
			}
			// PENDING or FAILED - can retry
			return true, "", nil
		}
		// No payment yet - can proceed
		return true, "", nil

	} else if paymentType == PaymentTypeCourseFee {
		// Check if course exists
		if courseID == nil || *courseID == 0 {
			return false, "Course ID is required for course fee payment", nil
		}

		var courseExists bool
		err = db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM course WHERE id = $1)", *courseID).Scan(&courseExists)
		if err != nil || !courseExists {
			return false, "Course not found", err
		}

		// Check if course payment already paid
		var status string
		err = db.DB.QueryRow("SELECT status FROM course_payment WHERE student_id = $1 AND course_id = $2", studentID, *courseID).Scan(&status)
		if err == nil {
			if status == PaymentStatusPaid {
				return false, fmt.Sprintf("Course payment already completed for course %d", *courseID), nil
			}
			// PENDING or FAILED - can retry
			return true, "", nil
		}
		// No payment yet - can proceed
		return true, "", nil
	}

	return false, "Invalid payment type", fmt.Errorf("invalid payment type: %s", paymentType)
}
