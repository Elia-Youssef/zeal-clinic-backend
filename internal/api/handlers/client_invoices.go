package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/validation"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetClientInvoices returns invoices where to_balance is a patient balance.
func GetClientInvoices(c echo.Context) error {
	items := store.InvoiceList{}
	if err := items.GetClientInvoices(c.Param("id")); err != nil {
		log.Println("Error: GetClientInvoices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch client invoices"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

// CreateClientInvoice creates an invoice for a patient.
// Direction: Self Balance (FROM) to Patient Balance (TO).
func CreateClientInvoice(c echo.Context) error {
	var req struct {
		PatientID  string              `json:"patientId"`
		CurrencyID string              `json:"currencyId"`
		Notes      string              `json:"notes"`
		Items      []store.InvoiceItem `json:"items"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
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
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	// Verify patient exists
	var patient store.Patient
	if err := patient.GetByID(req.PatientID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "patient not found"})
	}
	patientName := patient.FirstName + " " + patient.LastName

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := store.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientInvoice self balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve self balance"})
	}

	// Resolve patient balance (TO)
	patientBalance := store.Balance{
		EntityType: "patient",
		EntityID:   &req.PatientID,
		EntityName: patientName,
		CurrencyID: req.CurrencyID,
	}
	if err := patientBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientInvoice patient balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve patient balance"})
	}

	user := c.Get("user").(store.User)

	// Invoice.Create computes Amount (original total) and FinalAmount (discounted total) from items.
	inv := store.Invoice{
		FromBalanceID: selfBalance.ID,
		ToBalanceID:   patientBalance.ID,
		CurrencyID:    req.CurrencyID,
		Notes:         req.Notes,
		CreatedBy:     user.DisplayName,
		Items:         req.Items,
	}
	if err := inv.Create(); err != nil {
		log.Println("Error: CreateClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create invoice: " + err.Error()})
	}

	if selfBalance.EntityID != nil {
		inv.FromEntityID = *selfBalance.EntityID
	}
	if patientBalance.EntityID != nil {
		inv.ToEntityID = *patientBalance.EntityID
	}
	inv.FromEntityName = selfBalance.EntityName
	inv.ToEntityName = patientBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: inv})
}

func UpdateClientInvoice(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "invoiceNumber")
	inv := store.Invoice{ID: c.Param("id")}
	if err := inv.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: UpdateClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update invoice"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: inv})
}

func DeleteClientInvoice(c echo.Context) error {
	id := c.Param("id")
	inv := store.Invoice{ID: id}
	if err := inv.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: DeleteClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete invoice"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
