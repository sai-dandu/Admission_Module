package handlers

import (
	"admission-module/db"
	"admission-module/http/response"
	"admission-module/models"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// GetCourses retrieves all active courses
func GetCourses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	query := `SELECT id, name, description, fee, duration, is_active, created_at, updated_at FROM course WHERE is_active = 1 ORDER BY id ASC`
	rows, err := db.DB.QueryContext(r.Context(), query)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "Error fetching courses")
		return
	}
	defer rows.Close()

	courses := []models.Course{}
	for rows.Next() {
		var course models.Course
		if err := rows.Scan(&course.ID, &course.Name, &course.Description, &course.Fee, &course.Duration, &course.IsActive, &course.CreatedAt, &course.UpdatedAt); err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "Error processing courses")
			return
		}
		courses = append(courses, course)
	}

	if err = rows.Err(); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "Error processing courses")
		return
	}

	response.SuccessResponse(w, http.StatusOK, fmt.Sprintf("Retrieved %d courses", len(courses)), courses)
}

// GetCourseByID retrieves a specific course by ID
func GetCourseByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	courseIDStr := r.URL.Query().Get("id")
	if courseIDStr == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "Course ID is required")
		return
	}

	courseID, err := strconv.Atoi(courseIDStr)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "Invalid course ID")
		return
	}

	var course models.Course
	query := `SELECT id, name, description, fee, duration, is_active, created_at, updated_at FROM course WHERE id = $1`
	err = db.DB.QueryRowContext(r.Context(), query, courseID).Scan(&course.ID, &course.Name, &course.Description, &course.Fee, &course.Duration, &course.IsActive, &course.CreatedAt, &course.UpdatedAt)
	if err != nil {
		response.ErrorResponse(w, http.StatusNotFound, "Course not found")
		return
	}

	response.SuccessResponse(w, http.StatusOK, "Course retrieved", course)
}

// CreateCourse creates a new course (admin endpoint)
func CreateCourse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Fee         float64 `json:"fee"`
		Duration    string  `json:"duration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "Invalid request: "+err.Error())
		return
	}

	if req.Name == "" || req.Fee <= 0 {
		response.ErrorResponse(w, http.StatusBadRequest, "Name and fee are required")
		return
	}

	now := time.Now()
	var courseID int
	query := `INSERT INTO course (name, description, fee, duration, is_active, created_at, updated_at) VALUES ($1, $2, $3, $4, 1, $5, $6) RETURNING id`
	err := db.DB.QueryRowContext(r.Context(), query, req.Name, req.Description, req.Fee, req.Duration, now, now).Scan(&courseID)
	if err != nil {
		log.Printf("Error creating course: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "Error creating course")
		return
	}

	response.SuccessResponse(w, http.StatusCreated, "Course created successfully", map[string]interface{}{
		"course_id": courseID,
		"name":      req.Name,
		"fee":       req.Fee,
	})
}

// UpdateCourse updates an existing course (admin endpoint)
func UpdateCourse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		ID          int     `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Fee         float64 `json:"fee"`
		Duration    string  `json:"duration"`
		IsActive    bool    `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "Invalid request")
		return
	}

	if req.ID == 0 {
		response.ErrorResponse(w, http.StatusBadRequest, "Course ID is required")
		return
	}

	isActiveInt := 0
	if req.IsActive {
		isActiveInt = 1
	}

	query := `UPDATE course SET name = $1, description = $2, fee = $3, duration = $4, is_active = $5, updated_at = $6 WHERE id = $7`
	result, err := db.DB.ExecContext(r.Context(), query, req.Name, req.Description, req.Fee, req.Duration, isActiveInt, time.Now(), req.ID)
	if err != nil {
		log.Printf("Error updating course: %v", err)
		response.ErrorResponse(w, http.StatusInternalServerError, "Error updating course")
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "Error checking update")
		return
	}

	if rowsAffected == 0 {
		response.ErrorResponse(w, http.StatusNotFound, "Course not found")
		return
	}

	response.SuccessResponse(w, http.StatusOK, "Course updated successfully", map[string]interface{}{
		"course_id": req.ID,
	})
}
