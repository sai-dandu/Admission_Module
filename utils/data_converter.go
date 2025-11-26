package utils

import (
	"admission-module/models"
	"database/sql"
	"log"
)

// DeduplicateLeads removes duplicate leads within the same list based on email+phone combination
func DeduplicateLeads(leads []models.Lead) []models.Lead {
	seen := make(map[string]bool)
	unique := []models.Lead{}

	for _, lead := range leads {
		key := lead.Email + "|" + lead.Phone
		if !seen[key] {
			seen[key] = true
			unique = append(unique, lead)
		}
	}

	if len(unique) < len(leads) {
		log.Printf("Removed %d duplicate leads from collection", len(leads)-len(unique))
	}

	return unique
}

// ScanLead reads a single lead row from database query results
func ScanLead(rows *sql.Rows) (models.Lead, error) {
	var lead models.Lead
	var counsellorID sql.NullInt64
	var registrationPaymentID sql.NullInt64
	var selectedCourseID sql.NullInt64
	var coursePaymentID sql.NullInt64
	var interviewScheduledAt sql.NullTime

	err := rows.Scan(
		&lead.ID, &lead.Name, &lead.Email, &lead.Phone,
		&lead.Education, &lead.LeadSource, &counsellorID,
		&lead.MeetLink, &lead.ApplicationStatus,
		&registrationPaymentID, &selectedCourseID, &coursePaymentID, &interviewScheduledAt,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return lead, err
	}

	if counsellorID.Valid {
		lead.CounsellorID = &counsellorID.Int64
	}

	if registrationPaymentID.Valid {
		regPayID := int(registrationPaymentID.Int64)
		lead.RegistrationPaymentID = &regPayID
	}

	if selectedCourseID.Valid {
		selCourseID := int(selectedCourseID.Int64)
		lead.SelectedCourseID = &selCourseID
	}

	if coursePaymentID.Valid {
		coursePayID := int(coursePaymentID.Int64)
		lead.CoursePaymentID = &coursePayID
	}

	if interviewScheduledAt.Valid {
		lead.InterviewScheduledAt = &interviewScheduledAt.Time
	}

	return lead, nil
}

// ConvertLeadsToResponse converts slice of Lead to LeadResponse for API response
func ConvertLeadsToResponse(leads []models.Lead) []models.LeadResponse {
	responses := make([]models.LeadResponse, len(leads))
	for i := range leads {
		responses[i] = leads[i].ToResponse()
	}
	return responses
}
