package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"clinic-api/internal/validation"
	"database/sql"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

// GetSupplierInvoices returns invoices where from_balance is a supplier balance.
// Optional ?supplierBalanceId= filter.
func GetSupplierInvoices(c echo.Context) error {
	items, err := (&models.Invoice{}).GetSupplierInvoices(c.QueryParam("supplierBalanceId"))
	if err != nil {
		log.Println("Error: GetSupplierInvoices:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch supplier invoices"})
	}
	if items == nil {
		items = []models.Invoice{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetSupplierInvoiceByID(c echo.Context) error {
	var inv models.Invoice
	if err := inv.GetByID(c.Param("id")); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: GetSupplierInvoiceByID:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: inv})
}

// CreateSupplierInvoice records an invoice from a supplier (receiving goods).
// Direction: Supplier Balance (FROM) to Self Balance (TO).
func CreateSupplierInvoice(c echo.Context) error {
	var req struct {
		SupplierBalanceID string              `json:"supplierBalanceId"`
		SupplierName      string              `json:"supplierName"`
		CurrencyID        string              `json:"currencyId"`
		Notes             string              `json:"notes"`
		Items             []models.InvoiceItem `json:"items"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if req.SupplierBalanceID == "" && req.SupplierName == "" {
		errs["supplierName"] = "Supplier name is required for new suppliers"
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if len(req.Items) == 0 {
		errs["items"] = "At least one item is required"
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: errs})
	}

	// Resolve supplier balance (FROM)
	var supplierBalance models.Balance
	if req.SupplierBalanceID != "" {
		if err := supplierBalance.GetByID(req.SupplierBalanceID); err != nil {
			return c.JSON(http.StatusBadRequest, utils.Response{Error: "supplier balance not found"})
		}
		if supplierBalance.EntityType != "supplier" {
			return c.JSON(http.StatusBadRequest, utils.Response{Error: "balance is not a supplier balance"})
		}
	} else {
		entityID := uuid.Must(uuid.NewV7()).String()
		supplierBalance = models.Balance{
			EntityType: "supplier",
			EntityID:   &entityID,
			EntityName: req.SupplierName,
			CurrencyID: req.CurrencyID,
		}
		if err := supplierBalance.GetOrCreate(); err != nil {
			log.Println("Error: CreateSupplierInvoice supplier balance:", err)
			return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create supplier balance"})
		}
	}

	// Resolve self balance (TO)
	selfID := "self"
	selfBalance := models.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateSupplierInvoice self balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve self balance"})
	}

	// Compute total
	var total float64
	for _, item := range req.Items {
		total += item.Amount
	}

	user := c.Get("user").(models.User)

	inv := models.Invoice{
		FromBalanceID: supplierBalance.ID,
		ToBalanceID:   selfBalance.ID,
		Amount:        total,
		CurrencyID:    req.CurrencyID,
		Notes:         req.Notes,
		CreatedBy:     user.DisplayName,
		Items:         req.Items,
	}
	if err := inv.IsValid(); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed", Data: err})
	}
	if err := inv.Create(); err != nil {
		log.Println("Error: CreateSupplierInvoice:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create invoice: " + err.Error()})
	}

	inv.FromEntityName = supplierBalance.EntityName
	inv.ToEntityName = selfBalance.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: inv})
}

func UpdateSupplierInvoice(c echo.Context) error {
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
		log.Println("Error: UpdateSupplierInvoice:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: inv})
}

func DeleteSupplierInvoice(c echo.Context) error {
	id := c.Param("id")
	inv := models.Invoice{ID: id}
	if err := inv.Delete(); err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: DeleteSupplierInvoice:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
