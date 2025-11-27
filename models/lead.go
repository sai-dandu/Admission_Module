package models

import (
	"time"
)

// Lead represents a student lead
type Lead struct {
	ID                    int        `json:"id"`
	Name                  string     `json:"name"`
	Email                 string     `json:"email"`
	Phone                 string     `json:"phone"`
	Education             string     `json:"education"`
	LeadSource            string     `json:"lead_source"`
	CounsellorID          *int64     `json:"counsellor_id,omitempty"`
	MeetLink              string     `json:"meet_link"`
	ApplicationStatus     string     `json:"application_status"`
	RegistrationPaymentID *int       `json:"registration_payment_id,omitempty"`
	SelectedCourseID      *int       `json:"selected_course_id,omitempty"`
	CoursePaymentID       *int       `json:"course_payment_id,omitempty"`
	InterviewScheduledAt  *time.Time `json:"interview_scheduled_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// LeadResponse is the structured response for API responses
type LeadResponse struct {
	ID                   int     `json:"id"`
	Name                 string  `json:"name"`
	Email                string  `json:"email"`
	Phone                string  `json:"phone"`
	Education            string  `json:"education"`
	LeadSource           string  `json:"lead_source"`
	CounselorName        string  `json:"counselor_name"`
	MeetLink             string  `json:"meet_link"`
	ApplicationStatus    string  `json:"application_status"`
	SelectedCourseID     *int    `json:"selected_course_id,omitempty"`
	InterviewScheduledAt *string `json:"interview_scheduled_at,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

// ToResponse converts Lead to LeadResponse with formatted timestamps
func (l *Lead) ToResponse() LeadResponse {
	var scheduledAt *string
	if l.InterviewScheduledAt != nil {
		formatted := l.InterviewScheduledAt.Format(time.RFC3339)
		scheduledAt = &formatted
	}
	return LeadResponse{
		ID:                   l.ID,
		Name:                 l.Name,
		Email:                l.Email,
		Phone:                l.Phone,
		Education:            l.Education,
		LeadSource:           l.LeadSource,
		CounselorName:        "", // Will be populated by handler
		MeetLink:             l.MeetLink,
		ApplicationStatus:    l.ApplicationStatus,
		SelectedCourseID:     l.SelectedCourseID,
		InterviewScheduledAt: scheduledAt,
		CreatedAt:            l.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            l.UpdatedAt.Format(time.RFC3339),
	}
}
