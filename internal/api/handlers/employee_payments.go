package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
	"clinic-api/internal/validation"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetEmployeePayments returns every transaction between an employee and the
// clinic: payments, refunds, adjustments, and write-offs in both directions.
func GetEmployeePayments(c echo.Context) error {
	params := parseListParams(c)
	items := store.BalanceTransactionList{}
	total, err := items.GetEntityPayments("employee", c.Param("id"), params)
	if err != nil {
		log.Println("Error: GetEmployeePayments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch employee payments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

// CreateEmployeePayment records a payment from the clinic to an employee.
// Direction: Self Balance (FROM) to Employee Balance (TO).
func CreateEmployeePayment(c echo.Context) error {
	var req struct {
		EmployeeID        string  `json:"employeeId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateEmployeePayment] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.EmployeeID, "Employee ID"); msg != "" {
		errs["employeeId"] = msg
	}
	if msg := validation.Required(req.CurrencyID, "Currency ID"); msg != "" {
		errs["currencyId"] = msg
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if len(errs) > 0 {
		tracking.Warn(c, "[CreateEmployeePayment] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	// Verify currency exists
	var currency store.Currency
	if err := currency.GetByID(req.CurrencyID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "currency not found"})
	}

	// Verify employee exists
	var employee store.Employee
	if err := employee.GetByID(req.EmployeeID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "employee not found"})
	}
	employeeName := employee.FirstName + " " + employee.LastName

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := store.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateEmployeePayment self balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve self balance"})
	}

	// Resolve employee balance (TO)
	employeeBalance := store.Balance{
		EntityType: "employee",
		EntityID:   &req.EmployeeID,
		EntityName: employeeName,
		CurrencyID: req.CurrencyID,
	}
	if err := employeeBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateEmployeePayment employee balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve employee balance"})
	}

	user := c.Get("user").(store.User)

	bt := store.BalanceTransaction{
		FromBalanceID:     selfBalance.ID,
		ToBalanceID:       employeeBalance.ID,
		Amount:            req.Amount,
		CurrencyID:        req.CurrencyID,
		TransactionType:   "payment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateEmployeePayment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create payment: " + err.Error()})
	}

	bt.FromEntityName = selfBalance.EntityName
	bt.ToEntityName = employeeBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateEmployeeAdjustment records a manual balance correction for an employee.
// Direction is determined by `direction`: "incoming" = Employee to Self,
// "outgoing" = Self to Employee.
func CreateEmployeeAdjustment(c echo.Context) error {
	var req struct {
		EmployeeID        string  `json:"employeeId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Direction         string  `json:"direction"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateEmployeeAdjustment] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.EmployeeID, "Employee ID"); msg != "" {
		errs["employeeId"] = msg
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
		tracking.Warn(c, "[CreateEmployeeAdjustment] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	bt, err := createEntityBalanceCorrection(c, "employee", req.EmployeeID, req.CurrencyID,
		req.Amount, req.Direction, "adjustment", req.TransactionMethod, req.Description)
	if err != nil {
		log.Println("Error: CreateEmployeeAdjustment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create adjustment: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateEmployeeWriteOff records a debt write-off for an employee.
// Direction is determined by `direction`: "outgoing" = Self to Employee
// (forgive what we owe employee), "incoming" = Employee to Self.
func CreateEmployeeWriteOff(c echo.Context) error {
	var req struct {
		EmployeeID  string  `json:"employeeId"`
		Amount      float64 `json:"amount"`
		CurrencyID  string  `json:"currencyId"`
		Direction   string  `json:"direction"`
		Description string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateEmployeeWriteOff] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.EmployeeID, "Employee ID"); msg != "" {
		errs["employeeId"] = msg
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
		tracking.Warn(c, "[CreateEmployeeWriteOff] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}

	bt, err := createEntityBalanceCorrection(c, "employee", req.EmployeeID, req.CurrencyID,
		req.Amount, req.Direction, "write-off", "other", req.Description)
	if err != nil {
		log.Println("Error: CreateEmployeeWriteOff:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create write-off: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}
