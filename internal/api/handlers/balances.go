package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

func GetAllBalances(c echo.Context) error {
	entityType := c.QueryParam("entityType")
	items, err := (&models.Balance{}).GetAll(entityType)
	if err != nil {
		log.Println("Error: GetAllBalances failed to fetch balances")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch balances"})
	}
	if items == nil {
		items = []models.Balance{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetBalanceByID(c echo.Context) error {
	var item models.Balance
	if err := item.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: GetBalanceByID balance not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "balance not found"})
	} else if err != nil {
		log.Println("Error: GetBalanceByID failed to fetch balance")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch balance"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func GetOrCreateBalance(c echo.Context) error {
	var req struct {
		EntityType string `json:"entityType"`
		EntityID   string `json:"entityId"`
		EntityName string `json:"entityName"`
		Currency   string `json:"currency"`
	}
	if err := c.Bind(&req); err != nil {
		log.Println("Error: GetOrCreateBalance invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("entityType", req.EntityType, "Entity type")
	v.Required("entityId", req.EntityID, "Entity ID")
	v.OneOf("entityType", req.EntityType, []string{"patient", "employee", "self", "external"}, "Entity type")
	if v.HasErrors() {
		log.Println("Error: GetOrCreateBalance validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	balance := models.Balance{
		EntityType: req.EntityType,
		EntityID:   req.EntityID,
		EntityName: req.EntityName,
		Currency:   req.Currency,
	}
	if err := balance.GetOrCreate(); err != nil {
		log.Println("Error: GetOrCreateBalance failed to get/create balance")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to get/create balance"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: balance})
}

// Transactions
func CreateTransaction(c echo.Context) error {
	var bt models.BalanceTransaction
	if err := c.Bind(&bt); err != nil {
		log.Println("Error: CreateTransaction invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("debitBalanceId", bt.DebitBalanceID, "Debit balance ID")
	v.Required("creditBalanceId", bt.CreditBalanceID, "Credit balance ID")
	v.Positive("amount", bt.Amount, "Amount")
	if v.HasErrors() {
		log.Println("Error: CreateTransaction validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if bt.Currency == "" {
		bt.Currency = "USD"
	}
	if err := bt.Create(); err != nil {
		log.Println("Error: CreateTransaction failed to create transaction")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create transaction: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: bt})
}

func GetBalanceTransactions(c echo.Context) error {
	items, err := (&models.BalanceTransaction{}).GetByBalanceID(c.Param("id"))
	if err != nil {
		log.Println("Error: GetBalanceTransactions failed to fetch transactions")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch transactions"})
	}
	if items == nil {
		items = []models.BalanceTransaction{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetAllTransactions(c echo.Context) error {
	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	items, err := (&models.BalanceTransaction{}).GetAll(limit)
	if err != nil {
		log.Println("Error: GetAllTransactions failed to fetch transactions")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch transactions"})
	}
	if items == nil {
		items = []models.BalanceTransaction{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

// Invoices
func GetInvoices(c echo.Context) error {
	patientID := c.QueryParam("patientId")
	items, err := (&models.Invoice{}).GetAll(patientID)
	if err != nil {
		log.Println("Error: GetInvoices failed to fetch invoices")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch invoices"})
	}
	if items == nil {
		items = []models.Invoice{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetInvoiceByID(c echo.Context) error {
	var item models.Invoice
	if err := item.GetByID(c.Param("id")); err == sql.ErrNoRows {
		log.Println("Error: GetInvoiceByID invoice not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: GetInvoiceByID failed to fetch invoice")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: item})
}

func CreateInvoice(c echo.Context) error {
	var inv models.Invoice
	if err := c.Bind(&inv); err != nil {
		log.Println("Error: CreateInvoice invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v := NewValidator()
	v.Required("type", inv.Type, "Type")
	v.OneOf("type", inv.Type, []string{"invoice", "receipt", "expense"}, "Type")
	v.Positive("amount", inv.Amount, "Amount")
	if v.HasErrors() {
		log.Println("Error: CreateInvoice validation failed")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if inv.Status == "" {
		inv.Status = "pending"
	}
	if inv.Currency == "" {
		inv.Currency = "USD"
	}
	if inv.PaymentMethod == "" {
		inv.PaymentMethod = "cash"
	}
	if err := inv.Create(); err != nil {
		log.Println("Error: CreateInvoice failed to create invoice")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create invoice: " + err.Error()})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: inv})
}

func UpdateInvoice(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		log.Println("Error: UpdateInvoice invalid request")
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "invoiceNumber")
	inv := models.Invoice{ID: c.Param("id")}
	if err := inv.Update(updates); err == sql.ErrNoRows {
		log.Println("Error: UpdateInvoice invoice not found")
		return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice not found"})
	} else if err != nil {
		log.Println("Error: UpdateInvoice failed to update invoice")
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update invoice"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: inv})
}
