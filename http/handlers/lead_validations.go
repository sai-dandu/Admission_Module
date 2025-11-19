package handlers

import (
	"admission-module/models"
	"admission-module/utils"
)

// ValidateLead is a handler-level wrapper that delegates to utils.ValidateLead
// This maintains backward compatibility with the handler package
func ValidateLead(lead *models.Lead) error {
	return utils.ValidateLead(lead)
}
