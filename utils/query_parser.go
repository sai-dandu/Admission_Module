package utils

import (
	"fmt"
	"net/http"
	"time"
)

// TimeFilterParams holds parsed time filter parameters
type TimeFilterParams struct {
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}

// ParseTimeFilters extracts and validates time filter query parameters from HTTP request
func ParseTimeFilters(r *http.Request) (*TimeFilterParams, error) {
	params := &TimeFilterParams{}

	if str := r.URL.Query().Get("created_after"); str != "" {
		parsed, err := time.Parse(time.RFC3339, str)
		if err != nil {
			return nil, fmt.Errorf("invalid created_after format. Use RFC3339 (e.g., 2025-11-13T10:00:00Z)")
		}
		params.CreatedAfter = &parsed
	}

	if str := r.URL.Query().Get("created_before"); str != "" {
		parsed, err := time.Parse(time.RFC3339, str)
		if err != nil {
			return nil, fmt.Errorf("invalid created_before format. Use RFC3339 (e.g., 2025-11-13T10:00:00Z)")
		}
		params.CreatedBefore = &parsed
	}

	return params, nil
}
