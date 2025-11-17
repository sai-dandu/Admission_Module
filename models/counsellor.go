package models

type Counsellor struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	AssignedCount int    `json:"assigned_count"`
	MaxCapacity   int    `json:"max_capacity"`
}
