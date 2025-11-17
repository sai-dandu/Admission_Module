package handlers

import (
	"admission-module/db"
	"admission-module/http/services"
	"admission-module/models"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/razorpay/razorpay-go"
)

func InitiatePayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StudentID int     `json:"student_id"`
		Amount    float64 `json:"amount"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Basic validation
	if req.Amount <= 0 {
		http.Error(w, "Invalid amount", http.StatusBadRequest)
		return
	}

	// Ensure the student/lead exists before creating an order
	var exists bool
	err := db.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM leads WHERE id = $1)", req.StudentID).Scan(&exists)
	if err != nil {
		http.Error(w, "Error checking student", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Student not found", http.StatusBadRequest)
		return
	}

	// Get Razorpay credentials from environment variables
	keyID := os.Getenv("RazorpayKeyID")
	keySecret := os.Getenv("RazorpayKeySecret")

	if keyID == "" || keySecret == "" {
		http.Error(w, "Razorpay credentials not configured", http.StatusInternalServerError)
		return
	}

	client := razorpay.NewClient(keyID, keySecret)

	data := map[string]interface{}{
		"amount":   int(req.Amount * 100), // Convert to paise
		"currency": "INR",
		"receipt":  fmt.Sprintf("rcpt_%d", req.StudentID),
	}

	// Create Razorpay order
	resp, err := client.Order.Create(data, nil)
	if err != nil {
		http.Error(w, "Error creating order", http.StatusInternalServerError)
		return
	}

	orderID := resp["id"].(string)

	// Start a transaction
	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback() // Rollback if we don't commit

	// Save initial payment record
	_, err = tx.Exec("INSERT INTO payments (student_id, amount, status, order_id) VALUES ($1, $2, $3, $4)",
		req.StudentID, req.Amount, "PENDING", orderID)
	if err != nil {
		http.Error(w, "Error saving payment", http.StatusInternalServerError)
		return
	}

	// Update lead payment status to PENDING
	_, err = tx.Exec("UPDATE leads SET payment_status = 'PENDING' WHERE id = $1", req.StudentID)
	if err != nil {
		http.Error(w, "Error updating lead status", http.StatusInternalServerError)
		return
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		http.Error(w, "Error committing transaction", http.StatusInternalServerError)
		return
	}

	// Publish payment initiated event (best-effort)
	go func() {
		evt := map[string]interface{}{
			"event":      "payment.initiated",
			"student_id": req.StudentID,
			"order_id":   orderID,
			"amount":     req.Amount,
			"currency":   "INR",
			"status":     "PENDING",
			"ts":         time.Now().UTC().Format(time.RFC3339),
		}
		if err := services.Publish("payments", fmt.Sprintf("student-%d", req.StudentID), evt); err != nil {
			fmt.Printf("Warning: failed to publish payment.initiated event: %v\n", err)
		}
	}()

	// Return order details to client
	response := models.RazorpayOrder{
		OrderID:  orderID,
		Amount:   req.Amount,
		Currency: "INR",
		Receipt:  fmt.Sprintf("rcpt_%d", req.StudentID),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func VerifyPayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OrderID      string `json:"order_id"`
		PaymentID    string `json:"payment_id"`
		RazorpaySign string `json:"razorpay_signature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Start a transaction
	tx, err := db.DB.Begin()
	if err != nil {
		http.Error(w, "Error starting transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// Get student ID from payment record first
	var studentID int
	err = tx.QueryRow("SELECT student_id FROM payments WHERE order_id = $1", req.OrderID).Scan(&studentID)
	if err != nil {
		http.Error(w, "Error retrieving payment details", http.StatusInternalServerError)
		return
	}

	// Update payment status
	_, err = tx.Exec("UPDATE payments SET status = $1, payment_id = $2, razorpay_sign = $3 WHERE order_id = $4",
		"PAID", req.PaymentID, req.RazorpaySign, req.OrderID)
	if err != nil {
		http.Error(w, "Error updating payment", http.StatusInternalServerError)
		return
	}

	// Update lead status
	result, err := tx.Exec("UPDATE leads SET payment_status = 'PAID' WHERE id = $1", studentID)
	if err != nil {
		http.Error(w, "Error updating lead", http.StatusInternalServerError)
		return
	}

	// Check if lead was actually updated
	rows, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "Error checking lead update", http.StatusInternalServerError)
		return
	}
	if rows == 0 {
		http.Error(w, "Lead not found", http.StatusNotFound)
		return
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		fmt.Printf("Error committing transaction: %v\n", err)
		http.Error(w, "Error committing transaction", http.StatusInternalServerError)
		return
	}

	fmt.Printf("Payment verification successful - Student ID: %d, Order ID: %s\n", studentID, req.OrderID)

	// Publish payment verified event (best-effort)
	go func() {
		evt := map[string]interface{}{
			"event":      "payment.verified",
			"student_id": studentID,
			"order_id":   req.OrderID,
			"payment_id": req.PaymentID,
			"status":     "PAID",
			"ts":         time.Now().UTC().Format(time.RFC3339),
		}
		if err := services.Publish("payments", fmt.Sprintf("student-%d", studentID), evt); err != nil {
			fmt.Printf("Warning: failed to publish payment.verified event: %v\n", err)
		}
	}()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message":    "Payment verified successfully",
		"status":     "PAID",
		"student_id": fmt.Sprintf("%d", studentID),
		"order_id":   req.OrderID,
	})
}
