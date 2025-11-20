package utils

import (
	"admission-module/models"
	"context"
	"database/sql"
	"fmt"
	"log"
)

// LeadRepository handles all lead-related database operations
type LeadRepository struct {
	db *sql.DB
}

// NewLeadRepository creates a new lead repository instance
func NewLeadRepository(db *sql.DB) *LeadRepository {
	return &LeadRepository{db: db}
}

// ValidateLead validates all lead fields and returns comprehensive errors
func ValidateLead(lead *models.Lead) error {
	if err := ValidateName(lead.Name); err != nil {
		return err
	}

	if err := ValidateEmail(lead.Email); err != nil {
		return err
	}

	if err := ValidatePhone(lead.Phone); err != nil {
		return err
	}

	if err := ValidateEducation(lead.Education); err != nil {
		return err
	}

	if lead.LeadSource == "" {
		return fmt.Errorf("lead_source is required")
	}

	return nil
}

// LeadExists checks if a lead already exists by email or phone within a transaction
func LeadExists(ctx context.Context, tx *sql.Tx, email, phone string) (bool, error) {
	var count int
	query := "SELECT COUNT(*) FROM student_lead WHERE email = $1 OR phone = $2"
	err := tx.QueryRowContext(ctx, query, email, phone).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetAvailableCounselorID finds the best available counselor based on lead source
// This should be called within a transaction for consistency
func GetAvailableCounselorID(ctx context.Context, tx *sql.Tx, leadSource string) (*int64, error) {
	var query string

	// Route to appropriate counselor pool based on lead source
	switch leadSource {
	case "website":
		query = `SELECT id FROM counselor 
				 WHERE assigned_count < max_capacity 
				 ORDER BY assigned_count ASC, id ASC 
				 LIMIT 1 FOR UPDATE SKIP LOCKED`
	case "referral":
		query = `SELECT id FROM counselor 
				 WHERE is_referral_enabled = true 
				 AND assigned_count < max_capacity 
				 ORDER BY assigned_count ASC, id ASC 
				 LIMIT 1 FOR UPDATE SKIP LOCKED`
	default:
		query = `SELECT id FROM counselor 
				 WHERE assigned_count < max_capacity 
				 ORDER BY assigned_count ASC, id ASC 
				 LIMIT 1 FOR UPDATE SKIP LOCKED`
	}

	var counselorID int64
	err := tx.QueryRowContext(ctx, query).Scan(&counselorID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No available counselor
		}
		return nil, err
	}

	return &counselorID, nil
}

// InsertLead inserts a new lead record and returns the lead ID
func InsertLead(ctx context.Context, tx *sql.Tx, lead *models.Lead) (int64, error) {
	query := `
		INSERT INTO student_lead (
			name, email, phone, education, lead_source, 
			counselor_id, registration_fee_status, course_fee_status, meet_link, 
			application_status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id`

	var leadID int64
	err := tx.QueryRowContext(
		ctx,
		query,
		lead.Name,
		lead.Email,
		lead.Phone,
		lead.Education,
		lead.LeadSource,
		lead.CounsellorID,
		"PENDING",
		"PENDING",
		lead.MeetLink,
		lead.ApplicationStatus,
		lead.CreatedAt,
		lead.UpdatedAt,
	).Scan(&leadID)

	if err != nil {
		return 0, err
	}

	return leadID, nil
}

// UpdateCounselorAssignmentCount increments counselor's assignment count
func UpdateCounselorAssignmentCount(ctx context.Context, tx *sql.Tx, counselorID int64) error {
	query := "UPDATE counselor SET assigned_count = assigned_count + 1 WHERE id = $1"
	result, err := tx.ExecContext(ctx, query, counselorID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("counselor not found: %d", counselorID)
	}

	return nil
}

// GetCounselorNameByID fetches counselor name from database
func GetCounselorNameByID(ctx context.Context, db *sql.DB, counselorID *int64) string {
	if counselorID == nil {
		return "Not Assigned"
	}

	var name string
	query := "SELECT name FROM counselor WHERE id = $1"
	err := db.QueryRowContext(ctx, query, *counselorID).Scan(&name)
	if err != nil {
		log.Printf("Error fetching counselor name for ID %d: %v", *counselorID, err)
		return "Unknown"
	}
	return name
}
