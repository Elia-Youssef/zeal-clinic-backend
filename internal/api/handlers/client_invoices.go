package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"clinic-api/internal/validation"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetClientInvoices returns invoices where to_balance is a patient balance.
func GetClientInvoices(c echo.Context) error {
	items := models.InvoiceList{}
	if err := items.GetClientInvoices(c.Param("id")); err != nil {
		log.Println("Error: GetClientInvoices:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch client invoices"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetClientInvoiceByID(c echo.Context) error {
	var inv models.Invoice
	if err := inv.GetByID(c.Param("id")); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: GetClientInvoiceByID:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: inv})
}

// CreateClientInvoice creates an invoice for a patient.
// Direction: Self Balance (FROM) to Patient Balance (TO).
func CreateClientInvoice(c echo.Context) error {
	var req struct {
		PatientID  string               `json:"patientId"`
		CurrencyID string               `json:"currencyId"`
		Notes      string               `json:"notes"`
		Items      []models.InvoiceItem `json:"items"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	// Validate
	errs := make(validation.Errors)
	if msg := validation.Required(req.PatientID, "Patient ID"); msg != "" {
		errs["patientId"] = msg
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if len(req.Items) == 0 {
		errs["items"] = "At least one item is required"
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	// Verify patient exists
	var patient models.Patient
	if err := patient.GetByID(req.PatientID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "patient not found"})
	}
	patientName := patient.FirstName + " " + patient.LastName

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := models.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientInvoice self balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve self balance"})
	}

	// Resolve patient balance (TO)
	patientBalance := models.Balance{
		EntityType: "patient",
		EntityID:   &req.PatientID,
		EntityName: patientName,
		CurrencyID: req.CurrencyID,
	}
	if err := patientBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientInvoice patient balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve patient balance"})
	}

	user := c.Get("user").(models.User)

	// Invoice.Create computes Amount (original total) and FinalAmount (discounted total) from items.
	inv := models.Invoice{
		FromBalanceID: selfBalance.ID,
		ToBalanceID:   patientBalance.ID,
		CurrencyID:    req.CurrencyID,
		Notes:         req.Notes,
		CreatedBy:     user.DisplayName,
		Items:         req.Items,
	}
	if err := inv.Create(); err != nil {
		log.Println("Error: CreateClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create invoice: " + err.Error()})
	}

	inv.FromEntityName = selfBalance.EntityName
	inv.ToEntityName = patientBalance.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: inv})
}

func UpdateClientInvoice(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "invoiceNumber")
	inv := models.Invoice{ID: c.Param("id")}
	if err := inv.Update(updates); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: UpdateClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: inv})
}

func DeleteClientInvoice(c echo.Context) error {
	id := c.Param("id")
	inv := models.Invoice{ID: id}
	if err := inv.Delete(); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: DeleteClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
