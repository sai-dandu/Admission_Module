package models

import "time"

type Payment struct {
	ID           int       `json:"id"`
	StudentID    int       `json:"student_id"`
	Amount       float64   `json:"amount"`
	Status       string    `json:"status"`
	Timestamp    time.Time `json:"timestamp"`
	OrderID      string    `json:"order_id"`
	PaymentID    string    `json:"payment_id"`
	RazorpaySign string    `json:"razorpay_signature"`
}

type RazorpayOrder struct {
	OrderID  string  `json:"order_id"`
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Receipt  string  `json:"receipt"`
}
