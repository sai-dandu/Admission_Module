package utils

import (
	"fmt"
	"regexp"
)

// Email and phone regex patterns
var (
	EmailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	PhoneRegex = regexp.MustCompile(`^\+?[1-9]\d{1,14}$`)
)

// LeadValidationRules contains validation configuration
type LeadValidationRules struct {
	MaxNameLength      int
	MaxEducationLength int
}

// DefaultValidationRules provides default validation constraints
var DefaultValidationRules = LeadValidationRules{
	MaxNameLength:      100,
	MaxEducationLength: 200,
}

// ValidateEmail checks if email format is valid
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if !EmailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// ValidatePhone checks if phone is in E.164 format
func ValidatePhone(phone string) error {
	if phone == "" {
		return fmt.Errorf("phone is required")
	}
	if !PhoneRegex.MatchString(phone) {
		return fmt.Errorf("invalid phone format (use E.164 format, e.g., +919876543210)")
	}
	return nil
}

// ValidateName checks if name meets requirements
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > DefaultValidationRules.MaxNameLength {
		return fmt.Errorf("name must be less than %d characters", DefaultValidationRules.MaxNameLength)
	}
	return nil
}

// ValidateEducation checks if education meets requirements
func ValidateEducation(education string) error {
	if education != "" && len(education) > DefaultValidationRules.MaxEducationLength {
		return fmt.Errorf("education must be less than %d characters", DefaultValidationRules.MaxEducationLength)
	}
	return nil
}

// // ValidateLeadSource checks if lead source is valid
// func ValidateLeadSource(leadSource string) error {
// 	validSources := map[string]bool{
// 		SourceWebsite:  true,
// 		SourceReferral: true,
// 		SourceCampaign: true,
// 		GoogleAds:      true,
// 		FacebookAds:    true,

// 		"": true,
// 	}
// 	if !validSources[leadSource] {
// 		return fmt.Errorf("invalid lead source: %s", leadSource)
// 	}
// 	return nil
// }
