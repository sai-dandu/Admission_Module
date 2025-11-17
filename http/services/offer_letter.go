package services

import (
	"fmt"

	"github.com/jung-kurt/gofpdf"
)

// GenerateOfferLetter creates a PDF offer letter for an accepted student.
func GenerateOfferLetter(name, email string) (string, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, "Offer Letter")
	pdf.Ln(12)
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(40, 10, fmt.Sprintf("Dear %s,", name))
	pdf.Ln(12)
	pdf.Cell(40, 10, "Congratulations! You have been accepted.")
	pdf.Ln(12)
	pdf.Cell(40, 10, fmt.Sprintf("Email: %s", email))
	pdf.Ln(12)
	pdf.Cell(40, 10, "Best regards,")
	pdf.Ln(12)
	pdf.Cell(40, 10, "University Admin")

	fileName := fmt.Sprintf("offer_%s.pdf", name)
	err := pdf.OutputFileAndClose(fileName)
	if err != nil {
		return "", fmt.Errorf("error generating offer letter PDF: %w", err)
	}

	return fileName, nil
}
