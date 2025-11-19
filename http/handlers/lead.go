package handlers

import (
	"admission-module/db"
	resp "admission-module/http/response"
	"admission-module/models"
	"admission-module/services"
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

// LeadService handles all lead-related operations

type LeadService struct {
	db *sql.DB
}

// NewLeadService creates a new instance of LeadService with the provided database connection
func NewLeadService(database *sql.DB) *LeadService {
	return &LeadService{db: database}
}

// UploadLeads handles bulk lead upload via Excel file
// It processes the uploaded file, validates leads, removes duplicates, and stores them in the database
func (s *LeadService) UploadLeads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Extract and validate file upload
	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("Error getting form file: %v", err)
		respondError(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	log.Printf("Processing file upload: %s", header.Filename)

	// Create temporary file with automatic cleanup
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

	// Copy uploaded file to temp location
	if _, err = io.Copy(tempFile, file); err != nil {
		log.Printf("Error copying file: %v", err)
		respondError(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	if err := tempFile.Close(); err != nil {
		log.Printf("Error closing temp file: %v", err)
	}

	// Parse Excel file
	leads, err := services.ParseExcel(tempFilePath)
	if err != nil {
		log.Printf("Error parsing Excel: %v", err)
		respondError(w, "Error parsing Excel: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Remove duplicates within the uploaded file
	leads = utils.DeduplicateLeads(leads)

	// Process each lead and track results
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

	// Build response
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

// processAndInsertLead handles the complete lead creation workflow with transaction support
// It validates the lead, checks for duplicates, assigns a counselor, and inserts the lead record
func (s *LeadService) processAndInsertLead(ctx context.Context, lead *models.Lead) error {
	// Set timestamps
	now := time.Now()
	lead.CreatedAt = now
	lead.UpdatedAt = now

	// Validate lead data
	if err := utils.ValidateLead(lead); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Start database transaction
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check for duplicate lead
	exists, err := utils.LeadExists(ctx, tx, lead.Email, lead.Phone)
	if err != nil {
		return fmt.Errorf("error checking duplicate: %w", err)
	}
	if exists {
		return fmt.Errorf("lead already exists with this email or phone")
	}

	// Assign counselor if not already assigned
	if lead.CounsellorID == nil {
		counselorID, err := utils.GetAvailableCounselorID(ctx, tx, lead.LeadSource)
		if err != nil {
			return fmt.Errorf("error assigning counselor: %w", err)
		}
		lead.CounsellorID = counselorID

		if counselorID == nil {
			log.Printf("Warning: No available counselor for lead source: %s", lead.LeadSource)
		}
	}

	// Insert lead into database
	leadID, err := utils.InsertLead(ctx, tx, lead)
	if err != nil {
		return fmt.Errorf("error inserting lead: %w", err)
	}
	lead.ID = int(leadID)

	// Update counselor assignment count atomically
	if lead.CounsellorID != nil {
		if err := utils.UpdateCounselorAssignmentCount(ctx, tx, *lead.CounsellorID); err != nil {
			return fmt.Errorf("error updating counselor count: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Lead created successfully: ID=%d, Email=%s, Counselor=%v",
		leadID, lead.Email, lead.CounsellorID)

	// Send welcome email asynchronously (non-blocking)
	if err := services.SendWelcomeEmailWithCounselorInfo(ctx, lead); err != nil {
		log.Printf("Warning: failed to send welcome email: %v", err)
		// Don't fail the operation if email fails
	}

	return nil
}

// GetLeads retrieves leads with optional time-based filters
// It supports filtering by creation date and returns paginated results
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

	// Build dynamic query with filters
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

	// Add time-based filters dynamically
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

	// Scan results
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

// CreateLead handles single lead creation from JSON payload
// It validates the lead data, assigns a counselor, and stores it in the database
func (s *LeadService) CreateLead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Decode JSON request body
	var lead models.Lead
	if err := json.NewDecoder(r.Body).Decode(&lead); err != nil {
		log.Printf("Error decoding JSON: %v", err)
		respondError(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Process and insert lead
	if err := s.processAndInsertLead(ctx, &lead); err != nil {
		log.Printf("Error creating lead: %v", err)

		// Determine appropriate HTTP status code based on error type
		statusCode := http.StatusInternalServerError
		if err.Error() == "lead already exists with this email or phone" {
			statusCode = http.StatusConflict
		} else if len(err.Error()) > 10 && err.Error()[:10] == "validation" {
			statusCode = http.StatusBadRequest
		}

		respondError(w, err.Error(), statusCode)
		return
	}

	// Fetch counselor name for response
	counselorName := utils.GetCounselorNameByID(ctx, s.db, lead.CounsellorID)

	response := CreateLeadResponse{
		Message:       "Lead created successfully",
		StudentID:     int64(lead.ID),
		CounselorName: counselorName,
		Email:         lead.Email,
	}

	respondJSON(w, http.StatusCreated, response)
}

// Response types

// GetLeadsResponse represents the API response for retrieving leads
type GetLeadsResponse struct {
	Status  string                `json:"status"`
	Message string                `json:"message"`
	Count   int                   `json:"count"`
	Data    []models.LeadResponse `json:"data"`
}

// CreateLeadResponse represents the API response for creating a lead
type CreateLeadResponse struct {
	Message       string `json:"message"`
	StudentID     int64  `json:"student_id"`
	CounselorName string `json:"counselor_name"`
	Email         string `json:"email"`
}

// HTTP helper functions

// respondJSON sends a JSON response with the given status code
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	resp.SendJSON(w, status, data)
}

// respondError sends an error response with the given status code
func respondError(w http.ResponseWriter, message string, status int) {
	resp.ErrorResponse(w, status, message)
}

// Public handler wrappers for backward compatibility with existing routes

var service *LeadService

// InitHandlers initializes the lead service with database connection
func InitHandlers(database *sql.DB) {
	service = NewLeadService(database)
}

// UploadLeads is the public handler for uploading leads via Excel
func UploadLeads(w http.ResponseWriter, r *http.Request) {
	if service == nil {
		service = NewLeadService(db.DB)
	}
	service.UploadLeads(w, r)
}

// GetLeads is the public handler for retrieving leads
func GetLeads(w http.ResponseWriter, r *http.Request) {
	if service == nil {
		service = NewLeadService(db.DB)
	}
	service.GetLeads(w, r)
}

// CreateLead is the public handler for creating a single lead
func CreateLead(w http.ResponseWriter, r *http.Request) {
	if service == nil {
		service = NewLeadService(db.DB)
	}
	service.CreateLead(w, r)
}
