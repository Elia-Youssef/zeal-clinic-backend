package handlers

import (
	"clinic-api/internal/database/models"
	"fmt"

	"github.com/go-pdf/fpdf"
)

func generatePrescriptionPDF(rx models.Prescription, patient models.Patient) *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()

	// Header
	pdf.SetFillColor(30, 30, 30)
	pdf.Rect(0, 0, 210, 35, "F")

	pdf.SetTextColor(206, 140, 110) // copper #CE8C6E
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
	pdf.SetXY(120, 20)
	pdf.CellFormat(75, 4, "www.example.com", "", 0, "R", false, 0, "")

	// Title
	pdf.SetY(42)
	pdf.SetTextColor(30, 30, 30)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 8, "PRESCRIPTION", "", 1, "C", false, 0, "")

	// Thin copper line
	pdf.SetDrawColor(206, 140, 110)
	pdf.SetLineWidth(0.5)
	pdf.Line(15, pdf.GetY()+2, 195, pdf.GetY()+2)
	pdf.SetY(pdf.GetY() + 8)

	// Patient Info
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(100, 100, 100)
	y := pdf.GetY()

	patientName := patient.FirstName
	if patient.MiddleName != "" {
		patientName += " " + patient.MiddleName
	}
	patientName += " " + patient.LastName

	infoLeft := []string{
		fmt.Sprintf("Patient: %s", patientName),
		fmt.Sprintf("ID: %s", patient.ID),
		fmt.Sprintf("DOB: %s", patient.DateOfBirth),
	}
	infoRight := []string{
		fmt.Sprintf("Date: %s", rx.PrescriptionDate),
		fmt.Sprintf("Prescribed by: %s", rx.PrescribedBy),
		fmt.Sprintf("Status: %s", rx.Status),
	}

	for i, line := range infoLeft {
		pdf.SetXY(15, y+float64(i)*5)
		pdf.CellFormat(90, 5, line, "", 0, "L", false, 0, "")
	}
	for i, line := range infoRight {
		pdf.SetXY(110, y+float64(i)*5)
		pdf.CellFormat(85, 5, line, "", 0, "L", false, 0, "")
	}
	pdf.SetY(y + 20)

	// Medications Table
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(245, 245, 245)
	pdf.SetTextColor(30, 30, 30)

	colWidths := []float64{60, 30, 35, 25, 30}
	headers := []string{"Medication", "Dosage", "Frequency", "Duration", "Notes"}

	for i, h := range headers {
		pdf.CellFormat(colWidths[i], 8, h, "1", 0, "C", true, 0, "")
	}
	pdf.Ln(-1)

	pdf.SetFont("Helvetica", "", 9)
	for _, item := range rx.Items {
		vals := []string{item.Name, item.Dosage, item.Frequency, item.Duration, item.Notes}
		for i, v := range vals {
			pdf.CellFormat(colWidths[i], 7, v, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}

	// Instructions
	if rx.Instructions != "" {
		pdf.SetY(pdf.GetY() + 8)
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 6, "Instructions:", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		pdf.MultiCell(180, 5, rx.Instructions, "", "L", false)
	}

	// Signature Line
	pdf.SetY(pdf.GetY() + 20)
	pdf.SetDrawColor(180, 180, 180)
	pdf.Line(15, pdf.GetY(), 85, pdf.GetY())
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.SetY(pdf.GetY() + 2)
	pdf.CellFormat(70, 5, "Physician Signature", "", 0, "L", false, 0, "")

	// Footer
	pdf.SetY(270)
	pdf.SetDrawColor(206, 140, 110)
	pdf.SetLineWidth(0.3)
	pdf.Line(15, 270, 195, 270)
	pdf.SetFont("Helvetica", "", 7)
	pdf.SetTextColor(150, 150, 150)
	pdf.SetY(272)
	pdf.CellFormat(0, 4, "Zeal Clinic - Dr. Julian Vance - This prescription is digitally generated.", "", 0, "C", false, 0, "")

	return pdf
}
