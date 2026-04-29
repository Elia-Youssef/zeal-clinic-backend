package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/validation"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetSupplierPayments returns every transaction between a supplier and the
// clinic: payments, refunds, adjustments, and write-offs in both directions.
func GetSupplierPayments(c echo.Context) error {
	params := parseListParams(c)
	items := store.BalanceTransactionList{}
	total, err := items.GetEntityPayments("supplier", c.Param("id"), params)
	if err != nil {
		log.Println("Error: GetSupplierPayments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch supplier payments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

// CreateSupplierPayment records a payment from the clinic to a supplier.
// Direction: Self Balance (FROM) to Supplier Balance (TO).
func CreateSupplierPayment(c echo.Context) error {
	var req struct {
		SupplierID        string  `json:"supplierId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.SupplierID, "Supplier ID"); msg != "" {
		errs["supplierId"] = msg
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	// Look up supplier and resolve (or create) its balance
	var supplier store.Supplier
	if err := supplier.GetByID(req.SupplierID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "supplier not found"})
	}
	supplierBalance := store.Balance{
		EntityType: "supplier",
		EntityID:   &req.SupplierID,
		EntityName: supplier.Name,
		CurrencyID: req.CurrencyID,
	}
	if err := supplierBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateSupplierPayment supplier balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve supplier balance"})
	}

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := store.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateSupplierPayment self balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve self balance"})
	}

	user := c.Get("user").(store.User)

	bt := store.BalanceTransaction{
		FromBalanceID:     selfBalance.ID,
		ToBalanceID:       supplierBalance.ID,
		Amount:            req.Amount,
		CurrencyID:        req.CurrencyID,
		TransactionType:   "payment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateSupplierPayment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create payment: " + err.Error()})
	}

	bt.FromEntityName = selfBalance.EntityName
	bt.ToEntityName = supplierBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateSupplierAdjustment records a manual balance correction for a supplier.
// Direction is determined by `direction`: "incoming" = Supplier to Self,
// "outgoing" = Self to Supplier.
func CreateSupplierAdjustment(c echo.Context) error {
	var req struct {
		SupplierID        string  `json:"supplierId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Direction         string  `json:"direction"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.SupplierID, "Supplier ID"); msg != "" {
		errs["supplierId"] = msg
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if msg := validation.Required(req.Description, "Description"); msg != "" {
		errs["description"] = msg
	}
	if msg := validation.OneOf(req.Direction, []string{"incoming", "outgoing"}, "Direction"); msg != "" {
		errs["direction"] = msg
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	bt, err := createEntityBalanceCorrection(c, "supplier", req.SupplierID, req.CurrencyID,
		req.Amount, req.Direction, "adjustment", req.TransactionMethod, req.Description)
	if err != nil {
		log.Println("Error: CreateSupplierAdjustment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create adjustment: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateSupplierWriteOff records a debt write-off for a supplier.
// Direction is determined by `direction`: "outgoing" = Self to Supplier
// (forgive what we owe supplier), "incoming" = Supplier to Self.
func CreateSupplierWriteOff(c echo.Context) error {
	var req struct {
		SupplierID  string  `json:"supplierId"`
		Amount      float64 `json:"amount"`
		CurrencyID  string  `json:"currencyId"`
		Direction   string  `json:"direction"`
		Description string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.SupplierID, "Supplier ID"); msg != "" {
		errs["supplierId"] = msg
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if msg := validation.Required(req.Description, "Description"); msg != "" {
		errs["description"] = msg
	}
	if msg := validation.OneOf(req.Direction, []string{"incoming", "outgoing"}, "Direction"); msg != "" {
		errs["direction"] = msg
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	bt, err := createEntityBalanceCorrection(c, "supplier", req.SupplierID, req.CurrencyID,
		req.Amount, req.Direction, "write-off", "other", req.Description)
	if err != nil {
		log.Println("Error: CreateSupplierWriteOff:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create write-off: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}
