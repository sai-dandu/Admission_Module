package models

import (
	"encoding/json"
	"time"
)

// Lead represents a student lead. CounsellorID is a pointer so JSON
type Lead struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	Email             string    `json:"email"`
	Phone             string    `json:"phone"`
	Education         string    `json:"education"`
	LeadSource        string    `json:"lead_source"`
	CounsellorID      *int64    `json:"counsellor_id,omitempty"`
	PaymentStatus     string    `json:"payment_status"`
	MeetLink          string    `json:"meet_link"`
	ApplicationStatus string    `json:"application_status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// MarshalJSON customizes JSON output for Lead to format timestamps and reorder fields
func (l Lead) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"id":                 l.ID,
		"name":               l.Name,
		"email":              l.Email,
		"phone":              l.Phone,
		"education":          l.Education,
		"lead_source":        l.LeadSource,
		"counsellor_id":      l.CounsellorID,
		"payment_status":     l.PaymentStatus,
		"meet_link":          l.MeetLink,
		"application_status": l.ApplicationStatus,
		"created_at":         l.CreatedAt.Format("2006-01-02 15:04:05"),
		"updated_at":         l.UpdatedAt.Format("2006-01-02 15:04:05"),
	})
}

// LeadResponse is the structured response for API responses
type LeadResponse struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Email             string `json:"email"`
	Phone             string `json:"phone"`
	Education         string `json:"education"`
	LeadSource        string `json:"lead_source"`
	PaymentStatus     string `json:"payment_status"`
	MeetLink          string `json:"meet_link"`
	ApplicationStatus string `json:"application_status"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// ToResponse converts Lead to LeadResponse with formatted timestamps
func (l *Lead) ToResponse() LeadResponse {
	return LeadResponse{
		ID:                l.ID,
		Name:              l.Name,
		Email:             l.Email,
		Phone:             l.Phone,
		Education:         l.Education,
		LeadSource:        l.LeadSource,
		PaymentStatus:     l.PaymentStatus,
		MeetLink:          l.MeetLink,
		ApplicationStatus: l.ApplicationStatus,
		CreatedAt:         l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         l.UpdatedAt.Format(time.RFC3339),
	}
}
