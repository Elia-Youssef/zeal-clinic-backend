package handlers

import (
	"clinic-api/internal/database/models"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/go-pdf/fpdf"
)

func generateConsentPDF(consent models.ConsentForm, patient models.Patient) *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 25)
	pdf.AddPage()

	// Header
	pdf.SetFillColor(30, 30, 30)
	pdf.Rect(0, 0, 210, 35, "F")

	pdf.SetTextColor(206, 140, 110)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.SetXY(15, 8)
	pdf.CellFormat(80, 10, "ZEAL", "", 0, "L", false, 0, "")

	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetXY(15, 19)
	pdf.CellFormat(80, 5, "by Dr. Julian Vance", "", 0, "L", false, 0, "")
	pdf.SetXY(15, 25)
	pdf.CellFormat(80, 5, "Zeal Clinic", "", 0, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 8)
	pdf.SetXY(120, 10)
	pdf.CellFormat(75, 4, "Tel: +1 555 0100", "", 0, "R", false, 0, "")
	pdf.SetXY(120, 15)
	pdf.CellFormat(75, 4, "Email: info@example.com", "", 0, "R", false, 0, "")

	// Title
	pdf.SetY(42)
	pdf.SetTextColor(30, 30, 30)
	pdf.SetFont("Helvetica", "B", 14)
	title := "CONSENT FORM"
	if consent.ProcedureName != "" {
		title = fmt.Sprintf("CONSENT — %s", strings.ToUpper(consent.ProcedureName))
	}
	pdf.CellFormat(0, 8, title, "", 1, "C", false, 0, "")

	// Copper line
	pdf.SetDrawColor(206, 140, 110)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY()+2, 195, pdf.GetY()+2)
	pdf.SetY(pdf.GetY() + 8)

	// Patient Info
	patientName := patient.FirstName
	if patient.MiddleName != "" {
		patientName += " " + patient.MiddleName
	}
	patientName += " " + patient.LastName

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(100, 100, 100)
	pdf.CellFormat(95, 5, fmt.Sprintf("Patient: %s", patientName), "", 0, "L", false, 0, "")
	pdf.CellFormat(90, 5, fmt.Sprintf("ID: %s", patient.ID), "", 1, "L", false, 0, "")
	pdf.CellFormat(95, 5, fmt.Sprintf("DOB: %s", patient.DateOfBirth), "", 0, "L", false, 0, "")
	pdf.CellFormat(90, 5, fmt.Sprintf("Form Type: %s", consent.FormType), "", 1, "L", false, 0, "")
	pdf.SetY(pdf.GetY() + 8)

	// Consent Content
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(30, 30, 30)
	pdf.MultiCell(180, 5, consent.Content, "", "L", false)
	pdf.SetY(pdf.GetY() + 10)

	// Signature
	if consent.Status == "signed" {
		pdf.SetDrawColor(180, 180, 180)
		pdf.SetLineWidth(0.3)

		if consent.SignatureType == "drawn" && consent.SignatureData != "" {
			// Decode base64 signature and embed as image
			sigData := consent.SignatureData
			// Strip data URI prefix if present
			if idx := strings.Index(sigData, ","); idx >= 0 {
				sigData = sigData[idx+1:]
			}
			decoded, err := base64.StdEncoding.DecodeString(sigData)
			if err == nil {
				// Write to temp file for fpdf
				tmpFile := fmt.Sprintf("sig_%s.png", consent.ID)
				if err := os.WriteFile(tmpFile, decoded, 0644); err == nil {
					pdf.ImageOptions(tmpFile, 15, pdf.GetY(), 60, 20, false, fpdf.ImageOptions{ImageType: "PNG"}, 0, "")
					os.Remove(tmpFile)
					pdf.SetY(pdf.GetY() + 22)
				}
			}
		}

		pdf.Line(15, pdf.GetY(), 85, pdf.GetY())
		pdf.SetY(pdf.GetY() + 2)
		pdf.SetFont("Helvetica", "B", 9)
		pdf.CellFormat(70, 5, consent.SignedName, "", 0, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(0, 5, fmt.Sprintf("Signed: %s", consent.SignedAt), "", 1, "R", false, 0, "")
	} else {
		pdf.SetFont("Helvetica", "I", 9)
		pdf.SetTextColor(180, 180, 180)
		pdf.CellFormat(0, 5, "This consent form has not been signed yet.", "", 1, "L", false, 0, "")
	}

	// Footer
	pdf.SetY(270)
	pdf.SetDrawColor(206, 140, 110)
	pdf.SetLineWidth(0.3)
	pdf.Line(15, 270, 195, 270)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetY(272)
	pdf.CellFormat(0, 4, "Zeal Clinic - Dr. Julian Vance - Digital Consent Record", "", 0, "C", false, 0, "")

	return pdf
}
