package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllDiscounts(c echo.Context) error {
	params := parseListParams(c)
	items := store.DiscountList{}
	total, err := items.GetAll(c.QueryParam("type"), params)
	if err != nil {
		log.Println("Error: [GetAllDiscounts]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load discounts"})
	}
	if items == nil {
		items = []store.Discount{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetDiscountByID(c echo.Context) error {
	d := store.Discount{}
	if err := d.GetByID(c.Param("id")); err != nil {
		return storeError(c, err, "Discount not found", "Couldn't load discount")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: d})
}

// GetDiscountInvoices lists the invoices this discount was applied to: the
// invoices carrying it as an offer, plus (for a gift) the invoice that issued
// the gift card.
func GetDiscountInvoices(c echo.Context) error {
	params := parseListParams(c)
	items := store.InvoiceList{}
	total, err := items.GetByDiscount(c.Param("id"), params)
	if err != nil {
		log.Println("Error: [GetDiscountInvoices]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't load invoices"})
	}
	if items == nil {
		items = []store.Invoice{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func CreateDiscount(c echo.Context) error {
	var d store.Discount
	if err := c.Bind(&d); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	// Gift discounts may only be created via an invoice line.
	if d.DiscountType == "gift" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Gift cards can only be created through an invoice"})
	}
	if err := d.IsValid(); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
	}
	if err := d.Create(); err != nil {
		return storeError(c, err, "Discount not found", "Couldn't create discount")
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: d})
}

// RedeemGiftCode applies a gift card by code to a patient's balance. The
// gift's value is credited (patient.balance.amount goes negative, i.e. the
// patient is now owed that much by the clinic). Single-use.
func RedeemGiftCode(c echo.Context) error {
	var req struct {
		Code      string `json:"code"`
		PatientID string `json:"patientId"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	user := c.Get("user").(store.User)
	gift, err := store.ApplyGiftByCode(req.Code, req.PatientID, user.DisplayName)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "Gift card not found"})
		}
		log.Println("Error: [RedeemGiftCode]:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "This gift card can't be redeemed"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: gift})
}

func UpdateDiscount(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")
	delete(updates, "currentUsages")

	d := store.Discount{ID: c.Param("id")}
	if err := d.GetByID(d.ID); err != nil {
		return storeError(c, err, "Discount not found", "Couldn't update discount")
	}
	// Gift cards: only name & description are editable.
	if d.DiscountType == "gift" {
		allowed := map[string]any{}
		for _, k := range []string{"name", "description"} {
			if v, ok := updates[k]; ok {
				allowed[k] = v
			}
		}
		updates = allowed
	}

	if err := d.Update(updates); err != nil {
		return storeError(c, err, "Discount not found", "Couldn't update discount")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: d})
}

func DeleteDiscount(c echo.Context) error {
	d := store.Discount{ID: c.Param("id")}
	if store.HasDependencies(d.ID, map[string]string{"invoices": "discount_id", "invoice_items": "item_id"}) {
		return c.JSON(http.StatusConflict, httpx.Response{Error: "Can't delete discount while it's in use"})
	}
	if err := d.Delete(); err != nil {
		return storeError(c, err, "Discount not found", "Couldn't delete discount")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
