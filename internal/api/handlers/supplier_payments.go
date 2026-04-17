package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"clinic-api/internal/validation"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetSupplierPayments returns transactions FROM self balances TO supplier balances.
func GetSupplierPayments(c echo.Context) error {
	items := models.BalanceTransactionList{}
	err := items.GetSupplierPayments(c.Param("id"))
	if err != nil {
		log.Println("Error: GetSupplierPayments:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch supplier payments"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

// CreateSupplierPayment records a payment from the clinic to a supplier.
// Direction: Self Balance (FROM) to Supplier Balance (TO).
func CreateSupplierPayment(c echo.Context) error {
	var req struct {
		SupplierBalanceID string  `json:"supplierBalanceId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.SupplierBalanceID, "Supplier balance ID"); msg != "" {
		errs["supplierBalanceId"] = msg
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	// Verify supplier balance exists
	var supplierBalance models.Balance
	if err := supplierBalance.GetByID(req.SupplierBalanceID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "supplier balance not found"})
	}
	if supplierBalance.EntityType != "supplier" {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "balance is not a supplier balance"})
	}

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := models.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateSupplierPayment self balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve self balance"})
	}

	user := c.Get("user").(models.User)

	bt := models.BalanceTransaction{
		FromBalanceID:   selfBalance.ID,
		ToBalanceID:     supplierBalance.ID,
		Amount:          req.Amount,
		CurrencyID:      req.CurrencyID,
		TransactionType:   "payment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:       user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateSupplierPayment:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create payment: " + err.Error()})
	}

	bt.FromEntityName = selfBalance.EntityName
	bt.ToEntityName = supplierBalance.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
}
