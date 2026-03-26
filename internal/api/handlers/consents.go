package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Templates

func GetConsentTemplates(c echo.Context) error {
	list, err := (&models.ConsentTemplate{}).GetAll()
	if err != nil {
		log.Println("Error: [GetConsentTemplates] failed to fetch templates:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch templates"})
	}
	if list == nil {
		list = []models.ConsentTemplate{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: list})
}

func CreateConsentTemplate(c echo.Context) error {
	var t models.ConsentTemplate
	if err := c.Bind(&t); err != nil {
		log.Println("Error: [CreateConsentTemplate] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("name", t.Name, "Name")
	v.Required("type", t.Type, "Type")
	v.OneOf("type", t.Type, []string{"general", "procedure"}, "Type")
	v.Required("content", t.Content, "Content")
	if v.HasErrors() {
		log.Println("Error: [CreateConsentTemplate] validation failed:", v.Fields)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}
	t.IsActive = true

	if err := t.Create(); err != nil {
		log.Println("Error: [CreateConsentTemplate] failed to create template:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create template"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: t})
}

func DeleteConsentTemplate(c echo.Context) error {
	t := models.ConsentTemplate{ID: c.Param("id")}
	if err := t.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteConsentTemplate] template not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "template not found"})
	} else if err != nil {
		log.Println("Error: [DeleteConsentTemplate] failed to delete template:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete template"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// Consent Forms

func GetConsentsByPatient(c echo.Context) error {
	list, err := (&models.ConsentForm{}).GetByPatient(c.Param("patientId"))
	if err != nil {
		log.Println("Error: [GetConsentsByPatient] failed to fetch consents:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch consents"})
	}
	if list == nil {
		list = []models.ConsentForm{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: list})
}

func CreateConsent(c echo.Context) error {
	var f models.ConsentForm
	if err := c.Bind(&f); err != nil {
		log.Println("Error: [CreateConsent] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("patientId", f.PatientID, "Patient ID")
	v.Required("title", f.Title, "Title")
	v.Required("formType", f.FormType, "Form type")
	v.OneOf("formType", f.FormType, []string{"general", "procedure"}, "Form type")
	if v.HasErrors() {
		log.Println("Error: [CreateConsent] validation failed:", v.Fields)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}

	if err := f.Create(); err != nil {
		log.Println("Error: [CreateConsent] failed to create consent form:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create consent form"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: f})
}

func SignConsent(c echo.Context) error {
	var body struct {
		SignatureType string `json:"signatureType"`
		SignatureData string `json:"signatureData"`
		SignedName    string `json:"signedName"`
	}
	if err := c.Bind(&body); err != nil {
		log.Println("Error: [SignConsent] invalid request:", err)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("signedName", body.SignedName, "Signed name")
	v.Required("signatureType", body.SignatureType, "Signature type")
	v.OneOf("signatureType", body.SignatureType, []string{"drawn", "checkbox"}, "Signature type")
	if v.HasErrors() {
		log.Println("Error: [SignConsent] validation failed:", v.Fields)
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: v.Fields})
	}

	f := models.ConsentForm{ID: c.Param("id")}
	if err := f.Sign(body.SignatureType, body.SignatureData, body.SignedName); err != nil {
		log.Println("Error: [SignConsent] failed to sign consent:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to sign consent"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: f})
}

func RevokeConsent(c echo.Context) error {
	f := models.ConsentForm{ID: c.Param("id")}
	if err := f.Revoke(); err != nil {
		log.Println("Error: [RevokeConsent] failed to revoke consent:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to revoke consent"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

func DeleteConsent(c echo.Context) error {
	f := models.ConsentForm{ID: c.Param("id")}
	if err := f.Delete(); err == sql.ErrNoRows {
		log.Println("Error: [DeleteConsent] consent not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "consent not found"})
	} else if err != nil {
		log.Println("Error: [DeleteConsent] failed to delete consent:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete consent"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

func GenerateConsentPDF(c echo.Context) error {
	var f models.ConsentForm
	if err := f.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: [GenerateConsentPDF] consent form not found:", err)
		return c.JSON(http.StatusNotFound, utils.Response{Error: "consent form not found"})
	} else if err != nil {
		log.Println("Error: [GenerateConsentPDF] failed to fetch consent:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch consent"})
	}

	var patient models.Patient
	if err := patient.GetByID(f.PatientID); err != nil {
		log.Println("Error: [GenerateConsentPDF] failed to fetch patient:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch patient"})
	}

	pdf := generateConsentPDF(f, patient)
	c.Response().Header().Set("Content-Type", "application/pdf")
	c.Response().Header().Set("Content-Disposition", "inline; filename=consent-"+f.ID+".pdf")
	return pdf.Output(c.Response().Writer)
}
