package services

import (
	"admission-module/models"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ParseExcel reads Excel file and returns leads with flexible column detection
func ParseExcel(filePath string) ([]models.Lead, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Excel file: %w", err)
	}
	defer f.Close()

	// Get first available sheet
	sheetList := f.GetSheetList()
	if len(sheetList) == 0 {
		return nil, fmt.Errorf("no sheets found in Excel file")
	}
	sheetName := sheetList[0]
	fmt.Printf("[DEBUG] Parsing Excel sheet: %s\n", sheetName)

	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("no data in sheet")
	}

	// Auto-detect column order from headers
	headerRow := rows[0]
	fmt.Printf("[DEBUG] Headers: %v\n", headerRow)

	colIndices := detectColumns(headerRow)
	fmt.Printf("[DEBUG] Detected column indices: Name=%d, Email=%d, Phone=%d, Education=%d, LeadSource=%d\n",
		colIndices["name"], colIndices["email"], colIndices["phone"], colIndices["education"], colIndices["lead_source"])

	var leads []models.Lead

	for i := 1; i < len(rows); i++ {
		row := rows[i]

		// Skip empty rows
		if len(row) == 0 {
			fmt.Printf("[DEBUG] Row %d is empty, skipping\n", i+1)
			continue
		}

		// Extract fields using detected column indices
		name := extractField(row, colIndices["name"])
		email := extractField(row, colIndices["email"])
		phone := extractField(row, colIndices["phone"])
		education := extractField(row, colIndices["education"])
		leadSource := extractField(row, colIndices["lead_source"])

		fmt.Printf("[DEBUG] Row %d: Name=%s, Email=%s, Phone=%s, Education=%s, LeadSource=%s\n",
			i+1, name, email, phone, education, leadSource)

		// Validate required fields
		if name == "" || email == "" || phone == "" {
			fmt.Printf("[DEBUG] Row %d: missing required fields - Name=%q, Email=%q, Phone=%q\n", i+1, name, email, phone)
			continue
		}

		lead := models.Lead{
			Name:       name,
			Email:      email,
			Phone:      phone,
			Education:  education,
			LeadSource: leadSource,
		}

		// Default lead source if empty
		if lead.LeadSource == "" {
			lead.LeadSource = "website" // default source
			fmt.Printf("[DEBUG] Row %d: LeadSource was empty, setting default to 'website'\n", i+1)
		}

		fmt.Printf("[DEBUG] Row %d parsed successfully\n", i+1)
		leads = append(leads, lead)
	}
	return leads, nil
}

// detectColumns finds column indices by matching header names
func detectColumns(headers []string) map[string]int {
	indices := map[string]int{
		"name":        -1,
		"email":       -1,
		"phone":       -1,
		"education":   -1,
		"lead_source": -1,
	}

	for i, header := range headers {
		lower := strings.ToLower(strings.TrimSpace(header))

		// Strict matching for column names
		switch {
		case lower == "name" || lower == "student name" || lower == "full name":
			indices["name"] = i
		case lower == "email" || lower == "e-mail" || lower == "email address":
			indices["email"] = i
		case lower == "phone" || lower == "mobile" || lower == "phone number" || lower == "contact number":
			indices["phone"] = i
		case lower == "education" || lower == "qualification" || lower == "degree" || lower == "educational qualification":
			indices["education"] = i
		case lower == "lead_source" || lower == "lead source" || lower == "source":
			indices["lead_source"] = i
		}
	}

	fmt.Printf("[DEBUG] Column detection results: %v\n", indices)

	return indices
}

// extractField safely extracts a field from a row
func extractField(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}
