package handlers

import (
	"admission-module/db"
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
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT id, name, description, fee, duration, is_active, created_at, updated_at
		FROM course
		WHERE is_active = 1
		ORDER BY id ASC`

	rows, err := db.DB.QueryContext(r.Context(), query)
	if err != nil {
		log.Printf("Error fetching courses: %v", err)
		respondError(w, "Error fetching courses", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	courses := []models.Course{}
	for rows.Next() {
		var course models.Course
		err := rows.Scan(&course.ID, &course.Name, &course.Description, &course.Fee, &course.Duration, &course.IsActive, &course.CreatedAt, &course.UpdatedAt)
		if err != nil {
			log.Printf("Error scanning course: %v", err)
			respondError(w, "Error processing courses", http.StatusInternalServerError)
			return
		}
		courses = append(courses, course)
	}

	if err = rows.Err(); err != nil {
		log.Printf("Error iterating courses: %v", err)
		respondError(w, "Error processing courses", http.StatusInternalServerError)
		return
	}

	log.Printf("Retrieved %d courses", len(courses))
	for _, course := range courses {
		log.Printf("  - Course ID: %d, Name: %s, Fee: ₹%.2f", course.ID, course.Name, course.Fee)
	}

	response := map[string]interface{}{
		"status":  "success",
		"message": fmt.Sprintf("Retrieved %d courses", len(courses)),
		"count":   len(courses),
		"data":    courses,
	}
	respondJSON(w, http.StatusOK, response)
}

// GetCourseByID retrieves a specific course by ID
func GetCourseByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	courseIDStr := r.URL.Query().Get("id")
	if courseIDStr == "" {
		respondError(w, "Course ID is required", http.StatusBadRequest)
		return
	}

	courseID, err := strconv.Atoi(courseIDStr)
	if err != nil {
		respondError(w, "Invalid course ID", http.StatusBadRequest)
		return
	}

	var course models.Course
	query := `SELECT id, name, description, fee, duration, is_active, created_at, updated_at FROM course WHERE id = $1`
	err = db.DB.QueryRowContext(r.Context(), query, courseID).Scan(&course.ID, &course.Name, &course.Description, &course.Fee, &course.Duration, &course.IsActive, &course.CreatedAt, &course.UpdatedAt)
	if err != nil {
		log.Printf("Error fetching course: %v", err)
		respondError(w, "Course not found", http.StatusNotFound)
		return
	}

	log.Printf("Retrieved course - ID: %d, Name: %s, Fee: ₹%.2f", course.ID, course.Name, course.Fee)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "success",
		"data":   course,
	})
}

// CreateCourse creates a new course (admin endpoint)
func CreateCourse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Fee         float64 `json:"fee"`
		Duration    string  `json:"duration"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.Fee <= 0 {
		respondError(w, "Name and fee are required", http.StatusBadRequest)
		return
	}

	now := time.Now()
	var courseID int
	query := `
		INSERT INTO course (name, description, fee, duration, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, $5, $6)
		RETURNING id`

	err := db.DB.QueryRowContext(r.Context(), query, req.Name, req.Description, req.Fee, req.Duration, now, now).Scan(&courseID)
	if err != nil {
		log.Printf("Error creating course: %v", err)
		respondError(w, "Error creating course", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message":   "Course created successfully",
		"course_id": courseID,
		"name":      req.Name,
		"fee":       req.Fee,
	})
}

// UpdateCourse updates an existing course (admin endpoint)
func UpdateCourse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID          int     `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Fee         float64 `json:"fee"`
		Duration    string  `json:"duration"`
		IsActive    bool    `json:"is_active"` // false = inactive, true = active
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.ID == 0 {
		respondError(w, "Course ID is required", http.StatusBadRequest)
		return
	}

	query := `
		UPDATE course
		SET name = $1, description = $2, fee = $3, duration = $4, is_active = $5, updated_at = $6
		WHERE id = $7`

	isActiveInt := 0
	if req.IsActive {
		isActiveInt = 1
	}
	result, err := db.DB.ExecContext(r.Context(), query, req.Name, req.Description, req.Fee, req.Duration, isActiveInt, time.Now(), req.ID)
	if err != nil {
		log.Printf("Error updating course: %v", err)
		respondError(w, "Error updating course", http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		respondError(w, "Error checking update", http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		respondError(w, "Course not found", http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Course updated successfully",
		"course_id": req.ID,
	})
}
