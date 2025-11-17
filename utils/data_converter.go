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

	err := rows.Scan(
		&lead.ID, &lead.Name, &lead.Email, &lead.Phone,
		&lead.Education, &lead.LeadSource, &counsellorID,
		&lead.PaymentStatus, &lead.MeetLink, &lead.ApplicationStatus,
		&lead.CreatedAt, &lead.UpdatedAt,
	)
	if err != nil {
		return lead, err
	}

	if counsellorID.Valid {
		lead.CounsellorID = &counsellorID.Int64
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
