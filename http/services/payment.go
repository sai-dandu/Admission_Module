package services

import (
	"admission-module/db"
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
	PaymentTypCourseFee     = "COURSE_FEE"
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

	case PaymentTypCourseFee:
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

	log.Printf("[PAYMENT] Saving payment record - StudentID: %d, OrderID: %s, Amount: %.2f, Type: %s",
		studentID, orderID, req.Amount, req.PaymentType)

	if req.PaymentType == PaymentTypeRegistration {
		// Save to registration_payment table
		log.Printf("[PAYMENT] Inserting into registration_payment table - StudentID: %d", studentID)

		// Check if registration payment already exists
		var existingPaymentID int
		var existingStatus string
		err = tx.QueryRow("SELECT id, status FROM registration_payment WHERE student_id = $1", studentID).Scan(&existingPaymentID, &existingStatus)
		if err == nil {
			// Payment already exists - if PENDING, update with new order_id; if PAID, reject
			if existingStatus == "PAID" {
				log.Printf("[PAYMENT] Registration payment already paid - StudentID: %d, PaymentID: %d", studentID, existingPaymentID)
				tx.Rollback()
				return fmt.Errorf("registration payment already completed")
			}
			// Update existing PENDING payment with new order_id
			log.Printf("[PAYMENT] Updating existing PENDING registration payment - StudentID: %d, PaymentID: %d", studentID, existingPaymentID)
			_, err = tx.Exec("UPDATE registration_payment SET order_id = $1, amount = $2, updated_at = CURRENT_TIMESTAMP WHERE student_id = $3",
				orderID, req.Amount, studentID)
			if err != nil {
				log.Printf("[PAYMENT] Error updating registration payment: %v", err)
				return fmt.Errorf("error updating registration payment: %w", err)
			}
		} else {
			// No existing payment, insert new one
			_, err = tx.Exec(
				"INSERT INTO registration_payment (student_id, amount, status, order_id) VALUES ($1, $2, $3, $4)",
				studentID, req.Amount, "PENDING", orderID)
			if err != nil {
				log.Printf("[PAYMENT] Error saving registration payment: %v", err)
				return fmt.Errorf("error saving registration payment: %w", err)
			}
		}
		log.Printf("[PAYMENT] Registration payment record saved successfully")

		// Update student_lead registration_fee_status
		log.Printf("[PAYMENT] Updating student_lead registration_fee_status to PENDING - StudentID: %d", studentID)
		_, err = tx.Exec("UPDATE student_lead SET registration_fee_status = 'PENDING' WHERE id = $1", studentID)
		if err != nil {
			log.Printf("[PAYMENT] Error updating registration_fee_status: %v", err)
			return fmt.Errorf("error updating registration fee status: %w", err)
		}
		log.Printf("[PAYMENT] Registration fee status updated to PENDING")

	} else if req.PaymentType == PaymentTypCourseFee {
		// Save to course_payment table
		log.Printf("[PAYMENT] Inserting into course_payment table - StudentID: %d, CourseID: %d", studentID, *req.CourseID)

		// Check if course payment already exists for this student+course
		var existingPaymentID int
		var existingStatus string
		err = tx.QueryRow("SELECT id, status FROM course_payment WHERE student_id = $1 AND course_id = $2", studentID, *req.CourseID).Scan(&existingPaymentID, &existingStatus)
		if err == nil {
			// Payment already exists - if PENDING, update with new order_id; if PAID, reject
			if existingStatus == "PAID" {
				log.Printf("[PAYMENT] Course payment already paid - StudentID: %d, CourseID: %d, PaymentID: %d", studentID, *req.CourseID, existingPaymentID)
				tx.Rollback()
				return fmt.Errorf("course payment already completed")
			}
			// Update existing PENDING payment with new order_id
			log.Printf("[PAYMENT] Updating existing PENDING course payment - StudentID: %d, CourseID: %d, PaymentID: %d", studentID, *req.CourseID, existingPaymentID)
			_, err = tx.Exec("UPDATE course_payment SET order_id = $1, amount = $2, updated_at = CURRENT_TIMESTAMP WHERE student_id = $3 AND course_id = $4",
				orderID, req.Amount, studentID, *req.CourseID)
			if err != nil {
				log.Printf("[PAYMENT] Error updating course payment: %v", err)
				return fmt.Errorf("error updating course payment: %w", err)
			}
		} else {
			// No existing payment, insert new one
			_, err = tx.Exec(
				"INSERT INTO course_payment (student_id, course_id, amount, status, order_id) VALUES ($1, $2, $3, $4, $5)",
				studentID, *req.CourseID, req.Amount, "PENDING", orderID)
			if err != nil {
				log.Printf("[PAYMENT] Error saving course payment: %v", err)
				return fmt.Errorf("error saving course payment: %w", err)
			}
		}
		log.Printf("[PAYMENT] Course payment record saved successfully")

		// Update student_lead course_fee_status (only if column exists)
		log.Printf("[PAYMENT] Updating student_lead course_fee_status to PENDING - StudentID: %d", studentID)
		_, err = tx.Exec("UPDATE student_lead SET course_fee_status = 'PENDING' WHERE id = $1", studentID)
		if err != nil {
			// If column doesn't exist, try without it
			log.Printf("[PAYMENT] Error updating course_fee_status: %v (column may not exist yet)", err)
			// Don't fail - the migration might not have run yet
		} else {
			log.Printf("[PAYMENT] Course fee status updated to PENDING")
		}
	}

	// Commit transaction
	log.Printf("[PAYMENT] Committing SavePaymentRecord transaction...")
	if err = tx.Commit(); err != nil {
		log.Printf("[PAYMENT] Error committing SavePaymentRecord transaction: %v", err)
		return fmt.Errorf("error committing transaction")
	}
	log.Printf("[PAYMENT] SavePaymentRecord transaction committed successfully")

	log.Printf("Payment record saved - Student: %d, Order: %s, Type: %s, Amount: %.2f",
		studentID, orderID, req.PaymentType, req.Amount)

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
			log.Printf("Warning: failed to publish payment.initiated event: %v", err)
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

// VerifyPayment verifies and updates payment status
func (s *PaymentService) VerifyPayment(req VerifyPaymentRequest) (*VerifyPaymentResult, error) {
	tx, err := db.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("error starting transaction")
	}
	defer tx.Rollback()

	var studentID int
	var paymentType string
	var amount float64
	var courseID *int

	// Try registration_payment table first
	log.Printf("[PAYMENT] Looking up payment - OrderID: %s", req.OrderID)
	var regPaymentID int
	err = tx.QueryRow("SELECT id, student_id, amount FROM registration_payment WHERE order_id = $1", req.OrderID).Scan(&regPaymentID, &studentID, &amount)

	if err != nil {
		// If not found in registration_payment, check course_payment
		log.Printf("[PAYMENT] Not found in registration_payment, checking course_payment table - Error: %v", err)
		paymentType = PaymentTypCourseFee
		var coursePaymentID int
		err = tx.QueryRow(
			"SELECT id, student_id, course_id, amount FROM course_payment WHERE order_id = $1",
			req.OrderID,
		).Scan(&coursePaymentID, &studentID, &courseID, &amount)

		if err != nil {
			log.Printf("[PAYMENT] Payment lookup failed - OrderID: %s, Error: %v", req.OrderID, err)
			return nil, fmt.Errorf("payment not found for order_id: %s", req.OrderID)
		}
		log.Printf("[PAYMENT] Course payment found - StudentID: %d, CourseID: %v, Amount: %.2f", studentID, courseID, amount)

		// Update course_payment status
		log.Printf("[PAYMENT] Updating course_payment status to PAID - OrderID: %s", req.OrderID)
		_, err = tx.Exec(
			"UPDATE course_payment SET status = $1, payment_id = $2, razorpay_sign = $3, updated_at = CURRENT_TIMESTAMP WHERE order_id = $4",
			"PAID", req.PaymentID, req.RazorpaySign, req.OrderID)
		if err != nil {
			log.Printf("[PAYMENT] Error updating course_payment status: %v", err)
			return nil, fmt.Errorf("error updating course payment: %w", err)
		}

		// Update student_lead with selected course
		log.Printf("[PAYMENT] Updating student_lead for course payment - StudentID: %d, CourseID: %v", studentID, courseID)
		result, err := tx.Exec(
			"UPDATE student_lead SET course_fee_status = $1, selected_course_id = $2, course_payment_id = $3 WHERE id = $4",
			"PAID", courseID, coursePaymentID, studentID)
		if err != nil {
			log.Printf("[PAYMENT] Error updating student_lead: %v", err)
			return nil, fmt.Errorf("error updating lead: %w", err)
		}
		rows, _ := result.RowsAffected()
		log.Printf("[PAYMENT] Student_lead updated - Rows affected: %d", rows)

	} else {
		// Found in registration_payment
		paymentType = PaymentTypeRegistration
		log.Printf("[PAYMENT] Registration payment found - StudentID: %d, Amount: %.2f", studentID, amount)

		// Update registration_payment status
		log.Printf("[PAYMENT] Updating registration_payment status to PAID - OrderID: %s", req.OrderID)
		_, err = tx.Exec(
			"UPDATE registration_payment SET status = $1, payment_id = $2, razorpay_sign = $3, updated_at = CURRENT_TIMESTAMP WHERE order_id = $4",
			"PAID", req.PaymentID, req.RazorpaySign, req.OrderID)
		if err != nil {
			log.Printf("[PAYMENT] Error updating registration_payment status: %v", err)
			return nil, fmt.Errorf("error updating registration payment: %w", err)
		}

		// Update student_lead with registration payment ID and schedule meet
		log.Printf("[PAYMENT] Updating student_lead for registration payment - StudentID: %d, PaymentID: %d", studentID, regPaymentID)
		result, err := tx.Exec(
			"UPDATE student_lead SET registration_fee_status = $1, registration_payment_id = $2, interview_scheduled_at = CURRENT_TIMESTAMP + INTERVAL '1 hour', application_status = $3 WHERE id = $4",
			"PAID", regPaymentID, "INTERVIEW_SCHEDULED", studentID)
		if err != nil {
			log.Printf("[PAYMENT] Error updating student_lead: %v", err)
			return nil, fmt.Errorf("error updating lead: %w", err)
		}
		rows, _ := result.RowsAffected()
		log.Printf("[PAYMENT] Student_lead updated - Rows affected: %d, Meet scheduled automatically", rows)
	}

	// Commit transaction
	log.Printf("[PAYMENT] Committing transaction...")
	if err = tx.Commit(); err != nil {
		log.Printf("[PAYMENT] Error committing transaction: %v", err)
		return nil, fmt.Errorf("error committing transaction")
	}
	log.Printf("[PAYMENT] Transaction committed successfully - StudentID: %d", studentID)

	// Get student email
	var email string
	db.DB.QueryRow("SELECT email FROM student_lead WHERE id = $1", studentID).Scan(&email)

	log.Printf("Payment verified - Student: %d, Order: %s, Type: %s, Amount: %.2f", studentID, req.OrderID, paymentType, amount)

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
			log.Printf("Warning: failed to publish payment.verified event: %v", err)
		}
	}()
}

// IsRegistrationPayment checks if payment type is registration
func (s *PaymentService) IsRegistrationPayment(paymentType string) bool {
	return paymentType == PaymentTypeRegistration
}
