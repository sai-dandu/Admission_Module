package models

import "time"

type Payment struct {
	ID              int       `json:"id"`
	StudentID       int       `json:"student_id"`
	Amount          float64   `json:"amount"`
	Status          string    `json:"status"`
	PaymentType     string    `json:"payment_type"` // REGISTRATION or COURSE_FEE
	Timestamp       time.Time `json:"timestamp"`
	OrderID         string    `json:"order_id"`
	PaymentID       string    `json:"payment_id"`
	RazorpaySign    string    `json:"razorpay_signature"`
	RelatedCourseID *int      `json:"related_course_id,omitempty"`
}
