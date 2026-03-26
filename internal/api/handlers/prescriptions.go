package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetPrescriptionsByPatient(c echo.Context) error {
	list, err := (&models.Prescription{}).GetByPatient(c.Param("patientId"))
	if err != nil {
		log.Println("Error: [GetPrescriptionsByPatient] failed to fetch prescriptions:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch prescriptions"})
	}
	if list == nil {
		list = []models.Prescription{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: list})
}

func CreatePrescription(c echo.Context) error {
	var p models.Prescription
	if err := c.Bind(&p); err != nil {
		log.Println("Error: [CreatePrescription] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	v := NewValidator()
	v.Required("patientId", p.PatientID, "Patient ID")
	v.Required("prescriptionDate", p.PrescriptionDate, "Prescription date")
	v.Date("prescriptionDate", p.PrescriptionDate)
	if len(p.Items) == 0 {
		v.Fields["items"] = "at least one prescription item is required"
	}
	if v.HasErrors() {
		log.Println("Error: [CreatePrescription] validation failed:", v.Fields)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}

	if err := p.Create(); err != nil {
		log.Println("Error: [CreatePrescription] failed to create prescription:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create prescription"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: p})
}

func UpdatePrescription(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: [UpdatePrescription] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "patientId")
	delete(updates, "createdAt")

	p := models.Prescription{ID: c.Param("id")}
	if err := p.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: [UpdatePrescription] prescription not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "prescription not found"})
	} else if err != nil {
		log.Println("Error: [UpdatePrescription] failed to update prescription:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update prescription"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: p})
}

func DeletePrescription(c echo.Context) error {
	p := models.Prescription{ID: c.Param("id")}
	if err := p.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeletePrescription] prescription not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "prescription not found"})
	} else if err != nil {
		log.Println("Error: [DeletePrescription] failed to delete prescription:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete prescription"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// GeneratePrescriptionPDF creates a PDF for a prescription and returns it inline.
func GeneratePrescriptionPDF(c echo.Context) error {
	var p models.Prescription
	if err := p.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GeneratePrescriptionPDF] prescription not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "prescription not found"})
	} else if err != nil {
		log.Println("Error: [GeneratePrescriptionPDF] failed to fetch prescription:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch prescription"})
	}

	// Get patient info
	var patient models.Patient
	if err := patient.GetByID(p.PatientID); err != nil {
		log.Println("Error: [GeneratePrescriptionPDF] failed to fetch patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient"})
	}

	pdf := generatePrescriptionPDF(p, patient)

	c.Response().Header().Set("Content-Type", "application/pdf")
	c.Response().Header().Set("Content-Disposition", "inline; filename=prescription-"+p.ID+".pdf")
	return pdf.Output(c.Response().Writer)
}
