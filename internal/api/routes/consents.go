package routes

import (
	"clinic-api/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func SetupConsentRoutes(api *echo.Group) {
	// Consent templates
	api.GET("/consent-templates", handlers.GetConsentTemplates, scope("patients:read"))
	api.POST("/consent-templates", handlers.CreateConsentTemplate, scope("patients:write"))
	api.DELETE("/consent-templates/:id", handlers.DeleteConsentTemplate, scope("patients:delete"))

	// Consent forms
	api.GET("/patients/:patientId/consents", handlers.GetConsentsByPatient, scope("patients:read"))
	api.POST("/consents", handlers.CreateConsent, scope("patients:write"))
	api.PUT("/consents/:id/sign", handlers.SignConsent, scope("patients:write"))
	api.PUT("/consents/:id/revoke", handlers.RevokeConsent, scope("patients:write"))
	api.DELETE("/consents/:id", handlers.DeleteConsent, scope("patients:delete"))
	api.GET("/consents/:id/pdf", handlers.GenerateConsentPDF, scope("patients:read"))
}
