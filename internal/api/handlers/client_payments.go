package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/validation"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetClientPayments returns every transaction between a patient and the
// clinic: payments, refunds, adjustments, and write-offs in both directions.
func GetClientPayments(c echo.Context) error {
	params := parseListParams(c)
	items := store.BalanceTransactionList{}
	total, err := items.GetEntityPayments("patient", c.Param("id"), params)
	if err != nil {
		log.Println("Error: GetClientPayments:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch client payments"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

// CreateClientPayment records a payment from a patient.
// Direction: Patient Balance (FROM) to Self Balance (TO).
func CreateClientPayment(c echo.Context) error {
	var req struct {
		PatientID         string  `json:"patientId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.PatientID, "Patient ID"); msg != "" {
		errs["patientId"] = msg
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

	// Verify currency exists
	var currency store.Currency
	if err := currency.GetByID(req.CurrencyID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "currency not found"})
	}

	// Verify patient exists
	var patient store.Patient
	if err := patient.GetByID(req.PatientID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "patient not found"})
	}
	patientName := patient.FirstName + " " + patient.LastName

	// Resolve patient balance (FROM)
	patientBalance := store.Balance{
		EntityType: "patient",
		EntityID:   &req.PatientID,
		EntityName: patientName,
		CurrencyID: req.CurrencyID,
	}
	if err := patientBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientPayment patient balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve patient balance"})
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
		log.Println("Error: CreateClientPayment self balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve self balance"})
	}

	user := c.Get("user").(store.User)

	bt := store.BalanceTransaction{
		FromBalanceID:     patientBalance.ID,
		ToBalanceID:       selfBalance.ID,
		Amount:            req.Amount,
		CurrencyID:        req.CurrencyID,
		TransactionType:   "payment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateClientPayment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create payment: " + err.Error()})
	}

	bt.FromEntityName = patientBalance.EntityName
	bt.ToEntityName = selfBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateClientAdjustment records a manual balance correction for a patient.
// Direction is determined by `direction`: "incoming" = Patient to Self,
// "outgoing" = Self to Patient.
func CreateClientAdjustment(c echo.Context) error {
	var req struct {
		PatientID         string  `json:"patientId"`
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
	if msg := validation.Required(req.PatientID, "Patient ID"); msg != "" {
		errs["patientId"] = msg
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

	bt, err := createEntityBalanceCorrection(c, "patient", req.PatientID, req.CurrencyID,
		req.Amount, req.Direction, "adjustment", req.TransactionMethod, req.Description)
	if err != nil {
		log.Println("Error: CreateClientAdjustment:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create adjustment: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateClientWriteOff records a debt write-off for a patient.
// Direction is determined by `direction`: "incoming" = Patient to Self
// (forgive what patient owes), "outgoing" = Self to Patient (forgive a refund
// owed to patient).
func CreateClientWriteOff(c echo.Context) error {
	var req struct {
		PatientID   string  `json:"patientId"`
		Amount      float64 `json:"amount"`
		CurrencyID  string  `json:"currencyId"`
		Direction   string  `json:"direction"`
		Description string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.PatientID, "Patient ID"); msg != "" {
		errs["patientId"] = msg
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

	bt, err := createEntityBalanceCorrection(c, "patient", req.PatientID, req.CurrencyID,
		req.Amount, req.Direction, "write-off", "other", req.Description)
	if err != nil {
		log.Println("Error: CreateClientWriteOff:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create write-off: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}

// CreateClientRefund records a refund to a patient.
// Direction: Self Balance (FROM) to Patient Balance (TO).
func CreateClientRefund(c echo.Context) error {
	var req struct {
		PatientID         string  `json:"patientId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.PatientID, "Patient ID"); msg != "" {
		errs["patientId"] = msg
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

	var currency store.Currency
	if err := currency.GetByID(req.CurrencyID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "currency not found"})
	}

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
		log.Println("Error: CreateClientRefund self balance:", err)
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
		log.Println("Error: CreateClientRefund patient balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to resolve patient balance"})
	}

	user := c.Get("user").(store.User)

	bt := store.BalanceTransaction{
		FromBalanceID:     selfBalance.ID,
		ToBalanceID:       patientBalance.ID,
		Amount:            req.Amount,
		CurrencyID:        req.CurrencyID,
		TransactionType:   "refund",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateClientRefund:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create refund: " + err.Error()})
	}

	bt.FromEntityName = selfBalance.EntityName
	bt.ToEntityName = patientBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: bt})
}
