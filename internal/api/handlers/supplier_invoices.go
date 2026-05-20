package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
	"clinic-api/internal/validation"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetSupplierInvoices returns invoices where from_balance is a supplier balance.
func GetSupplierInvoices(c echo.Context) error {
	items := store.InvoiceList{}
	err := items.GetSupplierInvoices(c.Param("id"))
	if err != nil {
		log.Println("Error: GetSupplierInvoices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch supplier invoices"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

// CreateSupplierInvoice records an invoice from a supplier (receiving goods).
// Direction: Supplier Balance (FROM) to Self Balance (TO).
func CreateSupplierInvoice(c echo.Context) error {
	var req struct {
		SupplierBalanceID string              `json:"supplierBalanceId"`
		SupplierID        string              `json:"supplierId"`
		CurrencyID        string              `json:"currencyId"`
		Notes             string              `json:"notes"`
		InvoiceNumber     int                 `json:"invoiceNumber"`
		Items             []store.InvoiceItem `json:"items"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateSupplierInvoice] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if req.SupplierBalanceID == "" && req.SupplierID == "" {
		errs["supplierId"] = "Supplier ID is required"
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if len(req.Items) == 0 {
		errs["items"] = "At least one item is required"
	}
	if len(errs) > 0 {
		tracking.Warn(c, "[CreateSupplierInvoice] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	// Resolve supplier balance (FROM)
	var supplierBalance store.Balance
	if req.SupplierBalanceID != "" {
		if err := supplierBalance.GetByID(req.SupplierBalanceID); err != nil {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "supplier balance not found"})
		}
		if supplierBalance.EntityType != "supplier" {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "balance is not a supplier balance"})
		}
	} else {
		// Look up the supplier to get the name
		var supplier store.Supplier
		if err := supplier.GetByID(req.SupplierID); err != nil {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "supplier not found"})
		}
		supplierBalance = store.Balance{
			EntityType: "supplier",
			EntityID:   &req.SupplierID,
			EntityName: supplier.Name,
			CurrencyID: req.CurrencyID,
		}
		if err := supplierBalance.GetOrCreate(); err != nil {
			log.Println("Error: CreateSupplierInvoice supplier balance:", err)
			return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create supplier balance"})
		}
	}

	// Resolve self balance (TO)
	selfID := "self"
	selfBalance := store.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateSupplierInvoice self balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve self balance"})
	}

	user := c.Get("user").(store.User)

	inv := store.Invoice{
		FromBalanceID: supplierBalance.ID,
		ToBalanceID:   selfBalance.ID,
		CurrencyID:    req.CurrencyID,
		Notes:         req.Notes,
		InvoiceNumber: req.InvoiceNumber,
		CreatedBy:     user.DisplayName,
		Items:         req.Items,
	}
	if err := inv.IsValid(); err != nil {
		tracking.Warn(c, "[CreateSupplierInvoice] invoice IsValid failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	if err := inv.Create(); errors.Is(err, store.ErrConflict) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: err.Error()})
	} else if err != nil {
		log.Println("Error: CreateSupplierInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create invoice: " + err.Error()})
	}

	if supplierBalance.EntityID != nil {
		inv.FromEntityID = *supplierBalance.EntityID
	}
	if selfBalance.EntityID != nil {
		inv.ToEntityID = *selfBalance.EntityID
	}
	inv.FromEntityName = supplierBalance.EntityName
	inv.ToEntityName = selfBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: inv})
}

func UpdateSupplierInvoice(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		tracking.Warn(c, "[UpdateSupplierInvoice] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "invoiceNumber")
	inv := store.Invoice{ID: c.Param("id")}
	if err := inv.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: UpdateSupplierInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update invoice"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: inv})
}

// UpdateSupplierInvoiceItem sets a line item's amount and recomputes invoice
// totals + the charge balance transaction. Used to fill in or correct a price
// after the supplier invoice was recorded with a placeholder.
func UpdateSupplierInvoiceItem(c echo.Context) error {
	var req struct {
		Amount float64 `json:"amount"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[UpdateSupplierInvoiceItem] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		tracking.Warn(c, "[UpdateSupplierInvoiceItem] non-positive amount")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: msg})
	}
	inv := store.Invoice{ID: c.Param("id")}
	if err := inv.UpdateItemAmount(c.Param("itemId"), req.Amount); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "invoice or item not found"})
	} else if err != nil {
		log.Println("Error: UpdateSupplierInvoiceItem:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update invoice item"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: inv})
}

func DeleteSupplierInvoice(c echo.Context) error {
	id := c.Param("id")
	inv := store.Invoice{ID: id}
	if err := inv.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: DeleteSupplierInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete invoice"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
