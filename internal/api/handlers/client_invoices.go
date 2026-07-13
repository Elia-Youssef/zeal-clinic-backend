package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/monitor"
	"clinic-api/internal/tracking"
	"clinic-api/internal/validation"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// invoiceProductIDs returns the product item IDs from the given invoice items,
// used to scope low-stock checks to just the products an invoice touched.
func invoiceProductIDs(items store.InvoiceItemList) []string {
	var ids []string
	for _, it := range items {
		if it.ItemType == "product" && it.ItemID != "" {
			ids = append(ids, it.ItemID)
		}
	}
	return ids
}

// GetClientInvoices returns invoices where to_balance is a patient balance.
func GetClientInvoices(c echo.Context) error {
	params := parseListParams(c)
	items := store.InvoiceList{}
	total, err := items.GetClientInvoices(c.Param("id"), params)
	if err != nil {
		log.Println("Error: GetClientInvoices:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load client invoices"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

// CreateClientInvoice creates an invoice for a patient.
// Direction: Self Balance (FROM) to Patient Balance (TO).
//
// Optional fields:
//   - DiscountID: invoice-level "offer" discount, reduces final_amount.
//   - Items[i].ItemType == "gift" with GiftPatientID or GiftCode: creates a
//     gift discount during invoice creation; if GiftPatientID is set, the
//     gift's value is also auto-applied as a credit on that patient's balance.
func CreateClientInvoice(c echo.Context) error {
	var req struct {
		PatientID     string              `json:"patientId"`
		DiscountID    string              `json:"discountId"`
		Notes         string              `json:"notes"`
		InvoiceNumber int                 `json:"invoiceNumber"`
		Items         []store.InvoiceItem `json:"items"`
	}
	if err := c.Bind(&req); err != nil {
		tracking.Warn(c, "[CreateClientInvoice] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}

	// Validate
	errs := make(validation.Errors)
	if msg := validation.Required(req.PatientID, "Patient ID"); msg != "" {
		errs["patientId"] = msg
	}
	if len(req.Items) == 0 {
		errs["items"] = "At least one item is required"
	}
	for i, item := range req.Items {
		if msg := validation.Positive(float64(item.Quantity), "Quantity"); msg != "" {
			errs[fmt.Sprintf("items[%d].quantity", i)] = msg
		}
		if msg := validation.Positive(item.Amount, "Price"); msg != "" {
			errs[fmt.Sprintf("items[%d].amount", i)] = msg
		}
	}
	if len(errs) > 0 {
		tracking.Warn(c, "[CreateClientInvoice] validation failed")
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}

	// Verify patient exists
	var patient store.Patient
	if err := patient.GetByID(req.PatientID); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Patient not found"})
	}
	patientName := patient.FirstName + " " + patient.LastName
	currencyID := store.USDCurrencyID

	// Resolve self balance (FROM)
	selfID := "self"
	selfBalance := store.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: currencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientInvoice self balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load clinic balance"})
	}

	// Resolve patient balance (TO)
	patientBalance := store.Balance{
		EntityType: "patient",
		EntityID:   &req.PatientID,
		EntityName: patientName,
		CurrencyID: currencyID,
	}
	if err := patientBalance.GetOrCreate(); err != nil {
		log.Println("Error: CreateClientInvoice patient balance:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load patient balance"})
	}

	user := c.Get("user").(store.User)

	// Invoice.Create computes Amount (original total) and FinalAmount (discounted total) from items.
	// InvoiceNumber=0 lets Create auto-assign the next sequential number;
	// a positive value pins it (and bumps the sequence if higher than current max).
	inv := store.Invoice{
		FromBalanceID: selfBalance.ID,
		ToBalanceID:   patientBalance.ID,
		CurrencyID:    currencyID,
		DiscountID:    req.DiscountID,
		Notes:         req.Notes,
		InvoiceNumber: req.InvoiceNumber,
		CreatedBy:     user.DisplayName,
		Items:         req.Items,
	}
	if err := inv.Create(); errors.Is(err, store.ErrConflict) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: strings.TrimPrefix(err.Error(), store.ErrConflict.Error()+": ")})
	} else if errors.Is(err, store.ErrValidation) {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: strings.TrimPrefix(err.Error(), store.ErrValidation.Error()+": ")})
	} else if err != nil {
		log.Println("Error: CreateClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't create invoice"})
	}
	monitor.CheckLowStock(invoiceProductIDs(inv.Items))

	if selfBalance.EntityID != nil {
		inv.FromEntityID = *selfBalance.EntityID
	}
	if patientBalance.EntityID != nil {
		inv.ToEntityID = *patientBalance.EntityID
	}
	inv.FromEntityName = selfBalance.EntityName
	inv.ToEntityName = patientBalance.EntityName

	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: inv})
}

func UpdateClientInvoice(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		tracking.Warn(c, "[UpdateClientInvoice] bind failed: "+err.Error())
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "invoiceNumber")
	inv := store.Invoice{ID: c.Param("id")}
	if err := inv.Update(updates); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Invoice not found"})
	} else if err != nil {
		log.Println("Error: UpdateClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't update invoice"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: inv})
}

func DeleteClientInvoice(c echo.Context) error {
	id := c.Param("id")
	inv := store.Invoice{ID: id}
	if err := inv.GetByID(id); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Invoice not found"})
	} else if err != nil {
		log.Println("Error: DeleteClientInvoice fetch:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete invoice"})
	}
	productIDs := invoiceProductIDs(inv.Items)
	if err := inv.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Invoice not found"})
	} else if err != nil {
		log.Println("Error: DeleteClientInvoice:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete invoice"})
	}
	monitor.CheckLowStock(productIDs)
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
