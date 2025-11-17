package services

import (
	"admission-module/models"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// ParseExcel reads Sheet1 and returns leads. Columns assumed (indexing from 0):

func ParseExcel(filePath string) ([]models.Lead, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rows, err := f.GetRows("Sheet1")
	if err != nil {
		return nil, err
	}

	var leads []models.Lead
	for i, row := range rows {
		if i == 0 {
			continue // skip header
		}
		if len(row) < 10 {
			continue
		}
		lead := models.Lead{
			Name:              row[1],
			Email:             row[2],
			Phone:             row[3],
			Education:         row[4],
			LeadSource:        row[5],
			PaymentStatus:     row[7],
			MeetLink:          row[8],
			ApplicationStatus: row[9],
		}
		// Handle CounsellorID if present. Use pointer to int64 in model.
		if row[6] != "" {
			if id, err := strconv.Atoi(row[6]); err == nil {
				v := int64(id)
				lead.CounsellorID = &v
			} else {
				lead.CounsellorID = nil
			}
		} else {
			lead.CounsellorID = nil
		}
		leads = append(leads, lead)
	}
	return leads, nil
}
