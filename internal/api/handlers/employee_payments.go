package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"clinic-api/internal/validation"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetEmployeePayments returns transactions FROM self balances TO employee balances.
func GetEmployeePayments(c echo.Context) error {
	items := models.BalanceTransactionList{}
	err := items.GetEmployeePayments(c.Param("id"))
	if err != nil {
		log.Println("Error: GetEmployeePayments:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch employee payments"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

// CreateEmployeePayment records a payment from the clinic to an employee.
// Direction: Self Balance (FROM) to Employee Balance (TO).
func CreateEmployeePayment(c echo.Context) error {
	var req struct {
		EmployeeID      string  `json:"employeeId"`
		Amount          float64 `json:"amount"`
		CurrencyID      string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
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
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	// Verify currency exists
	var currency models.Currency
	if err := currency.GetByID(req.CurrencyID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "currency not found"})
	}

	// Verify employee exists
	var employee models.Employee
	if err := employee.GetByID(req.EmployeeID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "employee not found"})
	}
	employeeName := employee.FirstName + " " + employee.LastName

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := models.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: req.CurrencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateEmployeePayment self balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve self balance"})
	}

	// Resolve employee balance (TO)
	employeeBalance := models.Balance{
		EntityType: "employee",
		EntityID:   &req.EmployeeID,
		EntityName: employeeName,
		CurrencyID: req.CurrencyID,
	}
	if err := employeeBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateEmployeePayment employee balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve employee balance"})
	}

	user := c.Get("user").(models.User)

	bt := models.BalanceTransaction{
		FromBalanceID:   selfBalance.ID,
		ToBalanceID:     employeeBalance.ID,
		Amount:          req.Amount,
		CurrencyID:      req.CurrencyID,
		TransactionType:   "payment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:       user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateEmployeePayment:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create payment: " + err.Error()})
	}

	bt.FromEntityName = selfBalance.EntityName
	bt.ToEntityName = employeeBalance.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
}
