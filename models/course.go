package models

import "time"

// Course represents an academic course offered by the institution
type Course struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Fee         float64   `json:"fee"`
	Duration    string    `json:"duration"`
	IsActive    int       `json:"is_active"` // 0 = inactive, 1 = active
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CourseResponse is the structured response for API responses
type CourseResponse struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Fee         float64 `json:"fee"`
	Duration    string  `json:"duration"`
	IsActive    int     `json:"is_active"` // 0 = inactive, 1 = active
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

// ToResponse converts Course to CourseResponse with formatted timestamps
func (c *Course) ToResponse() CourseResponse {
	return CourseResponse{
		ID:          c.ID,
		Name:        c.Name,
		Description: c.Description,
		Fee:         c.Fee,
		Duration:    c.Duration,
		IsActive:    c.IsActive,
		CreatedAt:   c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   c.UpdatedAt.Format(time.RFC3339),
	}
}
