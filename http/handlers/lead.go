package handlers

import (
	"admission-module/db"
	resp "admission-module/http/response"
	"admission-module/services"
	"admission-module/models"
	"admission-module/utils"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// LeadService encapsulates lead management operations
type LeadService struct {
	db *sql.DB
}

func NewLeadService(database *sql.DB) *LeadService {
	return &LeadService{db: database}
}

// UploadLeads handles bulk lead upload via Excel file
func (s *LeadService) UploadLeads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("Error getting form file: %v", err)
		respondError(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	log.Printf("Processing file upload: %s", header.Filename)

	// Create temp file with proper cleanup
	tempFile, err := os.CreateTemp("", "leads_*.xlsx")
	if err != nil {
		log.Printf("Error creating temp file: %v", err)
		respondError(w, "Error processing file", http.StatusInternalServerError)
		return
	}
	tempFilePath := tempFile.Name()
	defer func() {
		tempFile.Close()
		os.Remove(tempFilePath)
	}()

	if _, err = io.Copy(tempFile, file); err != nil {
		log.Printf("Error copying file: %v", err)
		respondError(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	if err := tempFile.Close(); err != nil {
		log.Printf("Error closing temp file: %v", err)
	}

	leads, err := services.ParseExcel(tempFilePath)
	if err != nil {
		log.Printf("Error parsing Excel: %v", err)
		respondError(w, "Error parsing Excel: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Remove duplicates within the uploaded file
	leads = utils.DeduplicateLeads(leads)

	// Process leads with proper error tracking
	successCount := 0
	failedLeads := []map[string]string{}

	for i, lead := range leads {
		if err := s.processAndInsertLead(ctx, &lead); err != nil {
			log.Printf("Failed to process lead %d (%s): %v", i+1, lead.Email, err)
			failedLeads = append(failedLeads, map[string]string{
				"row":   fmt.Sprintf("%d", i+2), // +2 for header row
				"email": lead.Email,
				"phone": lead.Phone,
				"error": err.Error(),
			})
			continue
		}
		successCount++
	}

	log.Printf("Bulk upload completed: %d successful, %d failed", successCount, len(failedLeads))

	response := map[string]interface{}{
		"message":       fmt.Sprintf("Successfully uploaded %d leads", successCount),
		"success_count": successCount,
		"failed_count":  len(failedLeads),
		"total_count":   len(leads),
	}

	if len(failedLeads) > 0 {
		response["failed_leads"] = failedLeads
	}

	respondJSON(w, http.StatusOK, response)
}

// processAndInsertLead handles the complete lead creation process in a transaction
func (s *LeadService) processAndInsertLead(ctx context.Context, lead *models.Lead) error {
	// Validate lead data
	if err := validateLead(lead); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Start transaction
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check for duplicate (within transaction)
	exists, err := s.leadExistsTx(ctx, tx, lead.Email, lead.Phone)
	if err != nil {
		return fmt.Errorf("error checking duplicate: %w", err)
	}
	if exists {
		return fmt.Errorf("lead already exists with this email or phone")
	}

	// Assign counselor with row lock
	if err := s.assignCounselorTx(ctx, tx, lead); err != nil {
		return fmt.Errorf("error assigning counselor: %w", err)
	}

	// Insert lead
	leadID, err := s.insertLeadTx(ctx, tx, lead)
	if err != nil {
		return fmt.Errorf("error inserting lead: %w", err)
	}
	lead.ID = int(leadID)

	// Update counselor count atomically
	if lead.CounsellorID != nil {
		if err := s.updateCounselorCountTx(ctx, tx, *lead.CounsellorID); err != nil {
			return fmt.Errorf("error updating counselor count: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Lead created successfully: ID=%d, Email=%s, Counselor=%v",
		leadID, lead.Email, lead.CounsellorID)

	// Send welcome email with counselor information
	if err := services.SendWelcomeEmailWithCounselorInfo(ctx, lead); err != nil {
		log.Printf("Warning: failed to send welcome email: %v", err)
		// Don't return error
	}

	return nil
}

// GetLeads retrieves leads with optional filters
func (s *LeadService) GetLeads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Parse and validate query parameters
	timeParams, err := utils.ParseTimeFilters(r)
	if err != nil {
		respondError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build query with filters - Updated to use student_lead table
	query := `
		SELECT 
			id, name, email, phone, education, lead_source, 
			counselor_id, meet_link, 
			application_status, registration_payment_id, selected_course_id, 
			course_payment_id, interview_scheduled_at, created_at, updated_at 
		FROM student_lead 
		WHERE 1=1`

	args := []interface{}{}
	argCount := 0

	if timeParams.CreatedAfter != nil {
		argCount++
		query += fmt.Sprintf(" AND created_at >= $%d", argCount)
		args = append(args, *timeParams.CreatedAfter)
	}

	if timeParams.CreatedBefore != nil {
		argCount++
		query += fmt.Sprintf(" AND created_at <= $%d", argCount)
		args = append(args, *timeParams.CreatedBefore)
	}

	query += " ORDER BY id ASC"

	// Execute query
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		log.Printf("Error fetching leads: %v", err)
		respondError(w, "Error fetching leads", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	leads := []models.Lead{}
	for rows.Next() {
		lead, err := utils.ScanLead(rows)
		if err != nil {
			log.Printf("Error scanning lead: %v", err)
			respondError(w, "Error processing leads", http.StatusInternalServerError)
			return
		}
		leads = append(leads, lead)
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error iterating leads: %v", err)
		respondError(w, "Error processing leads", http.StatusInternalServerError)
		return
	}

	log.Printf("Retrieved %d leads", len(leads))

	// Convert leads to response format
	leadResponses := utils.ConvertLeadsToResponse(leads)

	response := GetLeadsResponse{
		Status:  "success",
		Message: fmt.Sprintf("Retrieved %d leads successfully", len(leads)),
		Count:   len(leads),
		Data:    leadResponses,
	}
	respondJSON(w, http.StatusOK, response)
}

// CreateLead handles single lead creation
func (s *LeadService) CreateLead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	var lead models.Lead
	if err := json.NewDecoder(r.Body).Decode(&lead); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		respondError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Process and insert lead
	if err := s.processAndInsertLead(ctx, &lead); err != nil {
		log.Printf("Error creating lead: %v", err)

		// Determine appropriate status code based on error
		statusCode := http.StatusInternalServerError
		if err.Error() == "lead already exists with this email or phone" {
			statusCode = http.StatusConflict
		} else if err.Error()[:10] == "validation" {
			statusCode = http.StatusBadRequest
		}

		respondError(w, err.Error(), statusCode)
		return
	}

	// Get counselor name for response
	counselorName := s.getCounselorName(ctx, lead.CounsellorID)

	response := CreateLeadResponse{
		Message:       "Lead created successfully",
		StudentID:     int64(lead.ID),
		CounselorName: counselorName,
		Email:         lead.Email,
	}

	respondJSON(w, http.StatusCreated, response)
}

// Transaction-safe helper methods

func (s *LeadService) assignCounselorTx(ctx context.Context, tx *sql.Tx, lead *models.Lead) error {
	if lead.CounsellorID != nil {
		return nil // Already assigned
	}

	counselorID, err := s.getAvailableCounselorIDTx(ctx, tx, lead.LeadSource)
	if err != nil {
		return err
	}

	lead.CounsellorID = counselorID

	if counselorID == nil {
		log.Printf("Warning: No available counselor for lead source: %s", lead.LeadSource)
	}

	return nil
}

func (s *LeadService) getAvailableCounselorIDTx(ctx context.Context, tx *sql.Tx, leadSource string) (*int64, error) {
	var query string

	switch leadSource {
	case utils.SourceWebsite:
		query = `SELECT id FROM counselor 
				 WHERE assigned_count < max_capacity 
				 ORDER BY assigned_count ASC, id ASC 
				 LIMIT 1 FOR UPDATE SKIP LOCKED`
	case utils.SourceReferral:
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

func (s *LeadService) leadExistsTx(ctx context.Context, tx *sql.Tx, email, phone string) (bool, error) {
	var count int
	query := "SELECT COUNT(*) FROM student_lead WHERE email = $1 OR phone = $2"
	err := tx.QueryRowContext(ctx, query, email, phone).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *LeadService) insertLeadTx(ctx context.Context, tx *sql.Tx, lead *models.Lead) (int64, error) {
	now := time.Now()

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
		now,
		now,
	).Scan(&leadID)

	if err != nil {
		return 0, err
	}

	return leadID, nil
}

func (s *LeadService) updateCounselorCountTx(ctx context.Context, tx *sql.Tx, counselorID int64) error {
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

func (s *LeadService) getCounselorName(ctx context.Context, counselorID *int64) string {
	if counselorID == nil {
		return "Not Assigned"
	}

	var name string
	query := "SELECT name FROM counselor WHERE id = $1"
	err := s.db.QueryRowContext(ctx, query, *counselorID).Scan(&name)
	if err != nil {
		log.Printf("Error fetching counselor name for ID %d: %v", *counselorID, err)
		return "Unknown"
	}
	return name
}

// Validation functions

func validateLead(lead *models.Lead) error {
	if err := utils.ValidateName(lead.Name); err != nil {
		return err
	}

	if err := utils.ValidateEmail(lead.Email); err != nil {
		return err
	}

	if err := utils.ValidatePhone(lead.Phone); err != nil {
		return err
	}

	if err := utils.ValidateEducation(lead.Education); err != nil {
		return err
	}

	// if err := utils.ValidateLeadSource(lead.LeadSource); err != nil {
	// 	return err
	// }

	return nil
}

// Helper functions

// Response types and helpers

type GetLeadsResponse struct {
	Status  string                `json:"status"`
	Message string                `json:"message"`
	Count   int                   `json:"count"`
	Data    []models.LeadResponse `json:"data"`
}

type CreateLeadResponse struct {
	Message       string `json:"message"`
	StudentID     int64  `json:"student_id"`
	CounselorName string `json:"counselor_name"`
	Email         string `json:"email"`
}

// Helper functions (wrappers around response package for backwards compatibility)
// These maintain the existing call pattern while delegating to the centralized response package

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	resp.SendJSON(w, status, data)
}

func respondError(w http.ResponseWriter, message string, status int) {
	resp.ErrorResponse(w, status, message)
}

// Public handler wrappers (for backward compatibility with existing routes)
var service *LeadService

func InitHandlers(database *sql.DB) {
	service = NewLeadService(database)
}

func UploadLeads(w http.ResponseWriter, r *http.Request) {
	if service == nil {
		service = NewLeadService(db.DB)
	}
	service.UploadLeads(w, r)
}

func GetLeads(w http.ResponseWriter, r *http.Request) {
	if service == nil {
		service = NewLeadService(db.DB)
	}
	service.GetLeads(w, r)
}

func CreateLead(w http.ResponseWriter, r *http.Request) {
	if service == nil {
		service = NewLeadService(db.DB)
	}
	service.CreateLead(w, r)
}
