package services

import (
	"admission-module/config"
	"admission-module/db"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// RazorpayWebhookPayload represents the structure of Razorpay webhook payload
type RazorpayWebhookPayload struct {
	ID        string                 `json:"id"`
	Event     string                 `json:"event"`
	CreatedAt int64                  `json:"created_at"`
	Contains  []string               `json:"contains"`
	Payload   map[string]interface{} `json:"payload"`
}

// VerifyWebhookSignature verifies the signature of the incoming webhook
func VerifyWebhookSignature(payload []byte, signature string) bool {
	webhookSecret := config.AppConfig.RazorpayWebhookSecret
	if webhookSecret == "" {
		return false
	}

	// Create HMAC-SHA256 signature
	h := hmac.New(sha256.New, []byte(webhookSecret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	// Compare signatures (constant-time comparison)
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

// RazorpayWebhookHandler handles incoming Razorpay webhooks
func RazorpayWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed"})
		return
	}

	// Read the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read request body"})
		return
	}
	defer r.Body.Close()

	// Get the signature from headers
	signature := r.Header.Get("X-Razorpay-Signature")
	if signature == "" {
		// For testing purposes, allow unsigned webhooks
		signature = "test_unsigned"
	}

	// Verify the signature
	signatureValid := VerifyWebhookSignature(bodyBytes, signature)

	// Parse the webhook payload
	var payload RazorpayWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid payload format"})
		return
	}

	log.Printf("[WEBHOOK] Received: %s", payload.Event)

	// Log the webhook to database
	if err := logWebhookToDB(payload, signature, signatureValid, ""); err != nil {
		log.Printf("[WEBHOOK] DB error: %v", err)
	}

	// Handle different webhook events
	switch payload.Event {
	case "payment.authorized":
		handlePaymentAuthorized(w, payload)
	case "payment.captured":
		handlePaymentCaptured(w, payload, signature)
	case "order.paid":
		handlePaymentCaptured(w, payload, signature)
	case "payment.failed":
		handlePaymentFailed(w, payload)
	case "payment.error":
		handlePaymentError(w, payload)
	default:
		// Acknowledge all webhooks even if we don't handle them
		log.Printf("ℹ️  [WEBHOOK] Unhandled event type: %s - acknowledging anyway", payload.Event)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "acknowledged", "event": payload.Event})
	}
}

// handlePaymentAuthorized handles payment.authorized event
func handlePaymentAuthorized(w http.ResponseWriter, payload RazorpayWebhookPayload) {
	// Extract order ID and payment ID
	data := payload.Payload
	_, ok := data["order"].(map[string]interface{})
	if !ok {
		// Try to get order_id directly
		if _, exists := data["order_id"]; exists {
			// order_id found
		}
	}

	log.Printf("Payment authorized: %+v", payload)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "processed", "event": "payment.authorized"})
}

// handlePaymentCaptured handles payment.captured event
// This is the critical event that confirms payment success
func handlePaymentCaptured(w http.ResponseWriter, payload RazorpayWebhookPayload, signature string) {
	// Extract payment info directly from map
	paymentMap, ok := payload.Payload["payment"].(map[string]interface{})
	if !ok {
		log.Printf("❌ Error: 'payment' key not found or invalid type in payload")
		log.Printf("DEBUG: Full payload: %+v", payload.Payload)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid payment data structure"})
		return
	}

	entityMap, ok := paymentMap["entity"].(map[string]interface{})
	if !ok {
		log.Printf("❌ Error: 'entity' key not found or invalid type in payment")
		log.Printf("DEBUG: Payment map: %+v", paymentMap)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid entity data structure"})
		return
	}

	// Extract required fields from entity
	paymentID, _ := entityMap["id"].(string)
	orderID, _ := entityMap["order_id"].(string)

	// Extract amount (could be float or int)
	var amount int64
	switch v := entityMap["amount"].(type) {
	case float64:
		amount = int64(v)
	case int:
		amount = int64(v)
	}

	if paymentID == "" || orderID == "" {
		log.Printf("[WEBHOOK] Error: Empty paymentID or orderID")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Missing payment_id or order_id"})
		return
	}

	log.Printf("Payment captured - Order: %s, Payment: %s, Amount: %d", orderID, paymentID, amount)

	// Process payment in transaction
	if err := processPaymentCaptured(orderID, paymentID, signature); err != nil {
		log.Printf("[WEBHOOK] Error: %v", err)
		// Update webhook processing status in database using webhook ID
		if updateErr := updateWebhookProcessingStatus(payload.ID, "FAILED", err.Error()); updateErr != nil {
			log.Printf("Error updating webhook status: %v", updateErr)
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Update webhook processing status as successful
	if updateErr := updateWebhookProcessingStatus(payload.ID, "COMPLETED", ""); updateErr != nil {
		log.Printf("[WEBHOOK] Status update error: %v", updateErr)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "processed",
		"event":      "payment.captured",
		"order_id":   orderID,
		"payment_id": paymentID,
	})
}

// handlePaymentFailed handles payment.failed event
func handlePaymentFailed(w http.ResponseWriter, payload RazorpayWebhookPayload) {
	// Extract payment info directly from map
	paymentMap, ok := payload.Payload["payment"].(map[string]interface{})
	if !ok {
		log.Printf("❌ Error: 'payment' key not found in failed payment webhook")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid payment data structure"})
		return
	}

	entityMap, ok := paymentMap["entity"].(map[string]interface{})
	if !ok {
		log.Printf("❌ Error: 'entity' key not found in failed payment")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid entity data structure"})
		return
	}

	// Extract required fields from entity
	paymentID, _ := entityMap["id"].(string)
	orderID, _ := entityMap["order_id"].(string)

	// Extract error details if present
	errorCode := ""
	errorDesc := ""
	if errMap, ok := entityMap["error"].(map[string]interface{}); ok {
		if code, ok := errMap["code"].(string); ok {
			errorCode = code
		}
		if desc, ok := errMap["description"].(string); ok {
			errorDesc = desc
		}
	}

	errorMsg := fmt.Sprintf("%s: %s", errorCode, errorDesc)

	log.Printf("[WEBHOOK] Payment failed: Order %s", orderID)

	// Update payment status to FAILED
	if err := updatePaymentStatusFailed(orderID, paymentID, errorMsg); err != nil {
		log.Printf("Error updating failed payment: %v", err)
		if updateErr := updateWebhookProcessingStatus(payload.ID, "FAILED", err.Error()); updateErr != nil {
			log.Printf("Error updating webhook status: %v", updateErr)
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Update webhook processing status
	if updateErr := updateWebhookProcessingStatus(payload.ID, "COMPLETED", ""); updateErr != nil {
		log.Printf("[WEBHOOK] Status update error: %v", updateErr)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "processed",
		"event":    "payment.failed",
		"order_id": orderID,
		"error":    errorMsg,
	})
}

// handlePaymentError handles payment.error event
func handlePaymentError(w http.ResponseWriter, payload RazorpayWebhookPayload) {
	log.Printf("Payment error webhook received: %+v", payload)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "acknowledged", "event": "payment.error"})
}

// processPaymentCaptured processes a successful payment capture
func processPaymentCaptured(orderID, paymentID, signature string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		log.Printf("[WEBHOOK] Transaction error: %v", err)
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer func() {
		if err != nil {
			log.Printf("  [WEBHOOK] Rolling back transaction due to error")
			tx.Rollback()
		}
	}()

	// First, determine which payment table this belongs to
	var studentID int
	var paymentType string
	var amount float64
	var currentStatus string

	// Try registration_payment first
	err = tx.QueryRow("SELECT student_id, amount, status FROM registration_payment WHERE order_id = $1", orderID).Scan(&studentID, &amount, &currentStatus)
	if err == nil {
		paymentType = PaymentTypeRegistration
		log.Printf("  ✓ [WEBHOOK] Found in registration_payment - StudentID: %d, Amount: ₹%.2f, Status: %s", studentID, amount, currentStatus)
	} else {
		log.Printf("  ✗ [WEBHOOK] Not found in registration_payment: %v", err)
		// Try course_payment
		var courseID int

		err = tx.QueryRow("SELECT student_id, course_id, amount, status FROM course_payment WHERE order_id = $1", orderID).Scan(&studentID, &courseID, &amount, &currentStatus)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Payment not found in ANY table for order_id: %s", orderID)
			log.Printf("    This means:")
			log.Printf("    1. Payment never initiated via /initiate-payment")
			log.Printf("    2. Order ID in webhook doesn't match database")
			log.Printf("    3. Payment already deleted")
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("  [WEBHOOK] Rollback error: %v", rollbackErr)
			}
			return fmt.Errorf("payment not found or already processed for order_id: %s", orderID)
		}
		paymentType = PaymentTypeCourseFee
		log.Printf("  ✓ [WEBHOOK] Found in course_payment - StudentID: %d, Amount: ₹%.2f, Status: %s", studentID, amount, currentStatus)
	}

	// Check if payment is already PAID (idempotency)
	if currentStatus == "PAID" {
		if err = tx.Commit(); err != nil {
			log.Printf("❌ [WEBHOOK] Error committing transaction: %v", err)
			return fmt.Errorf("error committing transaction: %w", err)
		}

		// Still publish the event in case it failed on the first webhook
		log.Printf("  [WEBHOOK] Publishing payment.verified event to Kafka (retry)...")
		publishPaymentVerifiedFromWebhook(studentID, orderID, paymentID, paymentType)

		// If registration payment, also schedule interview (retry)
		if paymentType == PaymentTypeRegistration {
			log.Printf("  [WEBHOOK] Scheduling interview after registration payment (retry)...")
			scheduleInterviewAfterPayment(studentID)
		}

		log.Printf("✅ [WEBHOOK] Successfully handled duplicate/retry webhook - OrderID: %s, PaymentID: %s, StudentID: %d", orderID, paymentID, studentID)
		return nil
	}

	log.Printf("  [WEBHOOK] Updating payment status to PAID (PaymentType: %s)", paymentType)

	// Update payment status to PAID
	if paymentType == PaymentTypeRegistration {
		log.Printf("    Updating registration_payment table...")
		_, err = tx.Exec(
			"UPDATE registration_payment SET status = $1, payment_id = $2, razorpay_sign = $3, updated_at = CURRENT_TIMESTAMP WHERE order_id = $4",
			"PAID", paymentID, signature, orderID)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Error updating registration payment status: %v", err)
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("  [WEBHOOK] Rollback error: %v", rollbackErr)
			}
			return fmt.Errorf("error updating registration payment status: %w", err)
		}

		// Get the registration_payment ID
		var registrationPaymentID int
		err = tx.QueryRow("SELECT id FROM registration_payment WHERE order_id = $1", orderID).Scan(&registrationPaymentID)
		if err != nil {
			log.Printf("  ⚠ [WEBHOOK] Warning retrieving registration_payment ID: %v", err)
		}

		log.Printf("    ✓ registration_payment updated")

		// Update student_lead registration_fee_status AND registration_payment_id
		log.Printf("    Updating student_lead registration_fee_status and registration_payment_id...")
		_, err = tx.Exec(
			"UPDATE student_lead SET registration_fee_status = 'PAID', registration_payment_id = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
			registrationPaymentID, studentID)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Error updating student registration fee status: %v", err)
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("  [WEBHOOK] Rollback error: %v", rollbackErr)
			}
			return fmt.Errorf("error updating student registration fee status: %w", err)
		} else {
			log.Printf("    ✓ student_lead registration_fee_status and registration_payment_id updated")
		}
	} else {
		log.Printf("    Updating course_payment table...")
		_, err = tx.Exec(
			"UPDATE course_payment SET status = $1, payment_id = $2, razorpay_sign = $3, updated_at = CURRENT_TIMESTAMP WHERE order_id = $4",
			"PAID", paymentID, signature, orderID)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Error updating course payment status: %v", err)
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("  [WEBHOOK] Rollback error: %v", rollbackErr)
			}
			return fmt.Errorf("error updating course payment status: %w", err)
		}

		// Get the course_payment ID
		var coursePaymentID int
		err = tx.QueryRow("SELECT id FROM course_payment WHERE order_id = $1", orderID).Scan(&coursePaymentID)
		if err != nil {
			log.Printf("  ⚠ [WEBHOOK] Warning retrieving course_payment ID: %v", err)
		}

		log.Printf("    ✓ course_payment updated")

		// Update student_lead course_fee_status AND course_payment_id
		log.Printf("    Updating student_lead course_fee_status and course_payment_id...")
		_, err = tx.Exec(
			"UPDATE student_lead SET course_fee_status = 'PAID', course_payment_id = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
			coursePaymentID, studentID)
		if err != nil {
			log.Printf("❌ [WEBHOOK] Error updating student course fee status: %v", err)
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				log.Printf("  [WEBHOOK] Rollback error: %v", rollbackErr)
			}
			return fmt.Errorf("error updating student course fee status: %w", err)
		} else {
			log.Printf("    ✓ student_lead course_fee_status and course_payment_id updated")
		}
	}

	log.Printf("  [WEBHOOK] Committing transaction...")
	if err = tx.Commit(); err != nil {
		log.Printf("❌ [WEBHOOK] Error committing transaction: %v", err)
		return fmt.Errorf("error committing transaction: %w", err)
	}
	log.Printf("  ✓ Transaction committed successfully")

	// Publish payment.verified event to Kafka (non-blocking)
	log.Printf("  [WEBHOOK] Publishing payment.verified event to Kafka...")
	publishPaymentVerifiedFromWebhook(studentID, orderID, paymentID, paymentType)

	// If registration payment, schedule interview
	if paymentType == PaymentTypeRegistration {
		log.Printf("  [WEBHOOK] Scheduling interview after registration payment...")
		scheduleInterviewAfterPayment(studentID)
	}

	log.Printf("✅ [WEBHOOK] Successfully processed payment captured - OrderID: %s, PaymentID: %s, StudentID: %d", orderID, paymentID, studentID)
	return nil
}

// updatePaymentStatusFailed updates payment status to FAILED
func updatePaymentStatusFailed(orderID, paymentID, errorMsg string) error {
	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Try to update registration_payment first
	result, err := tx.Exec(
		"UPDATE registration_payment SET status = $1, payment_id = $2, error_message = $3, updated_at = CURRENT_TIMESTAMP WHERE order_id = $4",
		"FAILED", paymentID, errorMsg, orderID)
	if err != nil {
		return fmt.Errorf("error updating registration payment: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error checking rows affected: %w", err)
	}

	// If no rows in registration_payment, try course_payment
	if rowsAffected == 0 {
		result, err = tx.Exec(
			"UPDATE course_payment SET status = $1, payment_id = $2, error_message = $3, updated_at = CURRENT_TIMESTAMP WHERE order_id = $4",
			"FAILED", paymentID, errorMsg, orderID)
		if err != nil {
			return fmt.Errorf("error updating course payment: %w", err)
		}

		rowsAffected, _ = result.RowsAffected()
		if rowsAffected == 0 {
			return fmt.Errorf("payment not found for order_id: %s", orderID)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

// logWebhookToDB logs the webhook event to database
func logWebhookToDB(payload RazorpayWebhookPayload, signature string, signatureValid bool, errorMsg string) error {
	payloadJSON, err := json.Marshal(payload.Payload)
	if err != nil {
		log.Printf("Error marshaling webhook payload: %v", err)
		return fmt.Errorf("error marshaling payload: %w", err)
	}

	webhookID := payload.ID
	if webhookID == "" {
		// Generate a unique ID if not provided (for test webhooks)
		webhookID = fmt.Sprintf("webhook_%d_%s", time.Now().UnixNano(), payload.Event)
	}

	// Log to razorpay_webhooks table - with ON CONFLICT for idempotency
	// Handles duplicate webhook_id (same webhook sent twice by Razorpay)
	_, err = db.DB.Exec(
		`INSERT INTO razorpay_webhooks (webhook_id, event_type, payload, status, retry_count, signature_valid)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (webhook_id) DO UPDATE
		 SET updated_at = CURRENT_TIMESTAMP, retry_count = razorpay_webhooks.retry_count + 1, signature_valid = EXCLUDED.signature_valid`,
		webhookID, payload.Event, string(payloadJSON), "RECEIVED", 0, signatureValid)

	if err != nil {
		log.Printf("❌ Error inserting webhook to database: %v", err)
		return fmt.Errorf("error inserting webhook: %w", err)
	}

	log.Printf("✓ Webhook logged to database with ID: %s", webhookID)
	return nil
}

// updateWebhookProcessingStatus updates the processing status of a webhook in database
func updateWebhookProcessingStatus(webhookID, processingStatus, errorMsg string) error {
	status := "PROCESSED"
	if processingStatus == "FAILED" {
		status = "FAILED"
	}

	// Truncate error message to avoid exceeding column length
	if len(errorMsg) > 500 {
		errorMsg = errorMsg[:500]
	}

	_, err := db.DB.Exec(
		"UPDATE razorpay_webhooks SET status = $1, processed_at = CURRENT_TIMESTAMP, error_message = $2 WHERE webhook_id = $3",
		status, errorMsg, webhookID)

	if err != nil {
		log.Printf("Error updating webhook processing status for webhook_id %s: %v", webhookID, err)
		return err
	}
	return nil
}

// publishPaymentVerifiedFromWebhook publishes payment.verified event to Kafka
func publishPaymentVerifiedFromWebhook(studentID int, orderID, paymentID, paymentType string) {
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
			log.Printf("Warning: failed to publish payment.verified event from webhook: %v", err)
		}
	}()
}

// scheduleInterviewAfterPayment schedules an interview after successful registration payment
func scheduleInterviewAfterPayment(studentID int) {
	go func() {
		// Get student details
		var name, email string
		err := db.DB.QueryRow("SELECT name, email FROM student_lead WHERE id = $1", studentID).Scan(&name, &email)
		if err != nil {
			log.Printf("Error fetching student details: %v", err)
			return
		}

		// Publish interview.schedule event
		evt := map[string]interface{}{
			"event":      "interview.schedule",
			"student_id": studentID,
			"name":       name,
			"email":      email,
			"ts":         time.Now().UTC().Format(time.RFC3339),
		}
		if err := Publish("interviews", fmt.Sprintf("student-%d", studentID), evt); err != nil {
			log.Printf("Warning: failed to publish interview.schedule event: %v", err)
		}
	}()
}
