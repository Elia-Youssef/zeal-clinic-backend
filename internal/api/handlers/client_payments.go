package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"clinic-api/internal/validation"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// GetClientPayments returns transactions FROM patient balances TO self balances.
func GetClientPayments(c echo.Context) error {
	items := models.BalanceTransactionList{}
	err := items.GetClientPayments(c.Param("id"))
	if err != nil {
		log.Println("Error: GetClientPayments:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch client payments"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
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
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
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
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	// Verify currency exists
	var currency models.Currency
	if err := currency.GetByID(req.CurrencyID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "currency not found"})
	}

	// Verify patient exists
	var patient models.Patient
	if err := patient.GetByID(req.PatientID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "patient not found"})
	}
	patientName := patient.FirstName + " " + patient.LastName

	// Resolve patient balance (FROM)
	patientBalance := models.Balance{
		EntityType: "patient",
		EntityID:   &req.PatientID,
		EntityName: patientName,
		CurrencyID: req.CurrencyID,
	}
	if err := patientBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientPayment patient balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve patient balance"})
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
		log.Println("Error: CreateClientPayment self balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve self balance"})
	}

	user := c.Get("user").(models.User)

	bt := models.BalanceTransaction{
		FromBalanceID:   patientBalance.ID,
		ToBalanceID:     selfBalance.ID,
		Amount:          req.Amount,
		CurrencyID:      req.CurrencyID,
		TransactionType:   "payment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:       user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateClientPayment:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create payment: " + err.Error()})
	}

	bt.FromEntityName = patientBalance.EntityName
	bt.ToEntityName = selfBalance.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
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
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
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
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	var currency models.Currency
	if err := currency.GetByID(req.CurrencyID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "currency not found"})
	}

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
		log.Println("Error: CreateClientRefund self balance:", err)
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
		log.Println("Error: CreateClientRefund patient balance:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to resolve patient balance"})
	}

	user := c.Get("user").(models.User)

	bt := models.BalanceTransaction{
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
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create refund: " + err.Error()})
	}

	bt.FromEntityName = selfBalance.EntityName
	bt.ToEntityName = patientBalance.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
}
