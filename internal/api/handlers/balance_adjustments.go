package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"clinic-api/internal/validation"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// CreateAdjustment records a manual balance adjustment (admin only).
// Can go in either direction depending on fromBalanceId/toBalanceId.
func CreateAdjustment(c echo.Context) error {
	var req struct {
		FromBalanceID     string  `json:"fromBalanceId"`
		ToBalanceID       string  `json:"toBalanceId"`
		Amount            float64 `json:"amount"`
		CurrencyID        string  `json:"currencyId"`
		TransactionMethod string  `json:"transactionMethod"`
		Description       string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.FromBalanceID, "From balance ID"); msg != "" {
		errs["fromBalanceId"] = msg
	}
	if msg := validation.Required(req.ToBalanceID, "To balance ID"); msg != "" {
		errs["toBalanceId"] = msg
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if msg := validation.Required(req.Description, "Description"); msg != "" {
		errs["description"] = msg
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	// Verify both balances exist
	var fromBal, toBal models.Balance
	if err := fromBal.GetByID(req.FromBalanceID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "from balance not found"})
	}
	if err := toBal.GetByID(req.ToBalanceID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "to balance not found"})
	}

	user := c.Get("user").(models.User)

	bt := models.BalanceTransaction{
		FromBalanceID:     req.FromBalanceID,
		ToBalanceID:       req.ToBalanceID,
		Amount:            req.Amount,
		CurrencyID:        req.CurrencyID,
		TransactionType:   "adjustment",
		TransactionMethod: req.TransactionMethod,
		Description:       req.Description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateAdjustment:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create adjustment: " + err.Error()})
	}

	bt.FromEntityName = fromBal.EntityName
	bt.ToEntityName = toBal.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
}

// CreateWriteOff records a debt write-off (admin only).
// Zeroes out uncollectible debt without actual money movement.
func CreateWriteOff(c echo.Context) error {
	var req struct {
		FromBalanceID string  `json:"fromBalanceId"`
		ToBalanceID   string  `json:"toBalanceId"`
		Amount        float64 `json:"amount"`
		CurrencyID    string  `json:"currencyId"`
		Description   string  `json:"description"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}

	errs := make(validation.Errors)
	if msg := validation.Required(req.FromBalanceID, "From balance ID"); msg != "" {
		errs["fromBalanceId"] = msg
	}
	if msg := validation.Required(req.ToBalanceID, "To balance ID"); msg != "" {
		errs["toBalanceId"] = msg
	}
	if msg := validation.Positive(req.Amount, "Amount"); msg != "" {
		errs["amount"] = msg
	}
	if msg := validation.Required(req.Description, "Description"); msg != "" {
		errs["description"] = msg
	}
	if len(errs) > 0 {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}

	var fromBal, toBal models.Balance
	if err := fromBal.GetByID(req.FromBalanceID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "from balance not found"})
	}
	if err := toBal.GetByID(req.ToBalanceID); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "to balance not found"})
	}

	user := c.Get("user").(models.User)

	bt := models.BalanceTransaction{
		FromBalanceID:     req.FromBalanceID,
		ToBalanceID:       req.ToBalanceID,
		Amount:            req.Amount,
		CurrencyID:        req.CurrencyID,
		TransactionType:   "write-off",
		TransactionMethod: "other",
		Description:       req.Description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateWriteOff:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create write-off: " + err.Error()})
	}

	bt.FromEntityName = fromBal.EntityName
	bt.ToEntityName = toBal.EntityName

	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
}
