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

// GetExpensePayments returns every transaction between an expense and the
// clinic: payments, refunds, adjustments, and write-offs in both directions.
func GetExpensePayments(c echo.Context) error {
	params := parseListParams(c)
	items := store.BalanceTransactionList{}
	total, err := items.GetEntityPayments("expense", c.Param("id"), params)
	if err != nil {
		log.Println("Error: GetExpensePayments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load expense payments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

// CreateExpensePayment records a payment from the clinic to an expense.
// Direction: Self Balance (FROM) to Expense Balance (TO).
func CreateExpensePayment(c echo.Context) error {
	var req struct {
		ExpenseID         string  `json:"expenseId"`
		Amount            float64 `json:"amount"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateExpensePayment] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.ExpenseID, "Expense ID"); msg != "" {
		errs["expenseId"] = msg
	}
	if msg := validation.GreaterThanZero(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if msg := validation.OneOf(req.TransactionMethod, []string{"cash", "card", "transfer", "discount", "other"}, "Method"); msg != "" {
		errs["transactionMethod"] = msg
	}
	if len(errs) > 0 {
		tracking.Warn(c, "[CreateExpensePayment] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	currencyID := store.USDCurrencyID

	// Verify expense exists
	var expense store.Expense
	if err := expense.GetByID(req.ExpenseID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Expense not found"})
	}

	// Resolve expense balance (TO)
	expenseBalance := store.Balance{
		EntityType: "expense",
		EntityID:   &req.ExpenseID,
		EntityName: expense.Name,
		CurrencyID: currencyID,
	}
	if err := expenseBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateExpensePayment expense balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load expense balance"})
	}

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := store.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: currencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateExpensePayment self balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load clinic balance"})
	}

	user := c.Get("user").(store.User)

	bt := store.BalanceTransaction{
		FromBalanceID:     selfBalance.ID,
		ToBalanceID:       expenseBalance.ID,
		Amount:            req.Amount,
		CurrencyID:        currencyID,
		TransactionType:   "payment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.CreateTwoWay(); err != nil {
		return storeError(c, err, "Transaction not found", "Couldn't create payment")
	}

	bt.FromEntityName = selfBalance.EntityName
	bt.ToEntityName = expenseBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateExpenseAdjustment records a manual balance correction for an expense.
// Direction is determined by `direction`: "incoming" = Expense to Self,
// "outgoing" = Self to Expense.
func CreateExpenseAdjustment(c echo.Context) error {
	var req struct {
		ExpenseID         string  `json:"expenseId"`
		Amount            float64 `json:"amount"`
		TransactionMethod string  `json:"transactionMethod"`
		Direction         string  `json:"direction"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateExpenseAdjustment] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.ExpenseID, "Expense ID"); msg != "" {
		errs["expenseId"] = msg
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
	if msg := validation.OneOf(req.TransactionMethod, []string{"cash", "card", "transfer", "discount", "other"}, "Method"); msg != "" {
		errs["transactionMethod"] = msg
	}
	if len(errs) > 0 {
		tracking.Warn(c, "[CreateExpenseAdjustment] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	currencyID := store.USDCurrencyID
	bt, err := createEntityBalanceCorrection(c, "expense", req.ExpenseID, currencyID,
		req.Amount, req.Direction, "adjustment", req.TransactionMethod, req.Description)
	if err != nil {
		return storeError(c, err, "Expense not found", "Couldn't create adjustment")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateExpenseWriteOff records a debt write-off for an expense.
// Direction is determined by `direction`: "outgoing" = Self to Expense,
// "incoming" = Expense to Self.
func CreateExpenseWriteOff(c echo.Context) error {
	var req struct {
		ExpenseID   string  `json:"expenseId"`
		Amount      float64 `json:"amount"`
		Direction   string  `json:"direction"`
		Description string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateExpenseWriteOff] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.ExpenseID, "Expense ID"); msg != "" {
		errs["expenseId"] = msg
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
		tracking.Warn(c, "[CreateExpenseWriteOff] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	currencyID := store.USDCurrencyID
	bt, err := createEntityBalanceCorrection(c, "expense", req.ExpenseID, currencyID,
		req.Amount, req.Direction, "write-off", "other", req.Description)
	if err != nil {
		return storeError(c, err, "Expense not found", "Couldn't create write-off")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}
