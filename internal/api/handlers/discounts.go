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
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllDiscounts]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch discounts"})
	}
	if items == nil {
		items = []store.Discount{}
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: httpx.PaginatedList{Items: items, Total: total}})
}

func GetDiscountByID(c echo.Context) error {
	d := store.Discount{}
	if err := d.GetByID(c.Param("id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "discount not found"})
		}
		log.Println("Error: [GetDiscountByID]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch discount"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: d})
}

func CreateDiscount(c echo.Context) error {
	var d store.Discount
	if err := c.Bind(&d); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	// Gift discounts may only be created via an invoice line.
	if d.DiscountType == "gift" {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "gift discounts can only be created through an invoice"})
	}
	if err := d.IsValid(); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "validation failed"})
	}
	if err := d.Create(); err != nil {
		log.Println("Error: [CreateDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to create discount"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: d})
}

// RedeemGiftCode applies a gift card by code to a patient's balance. The
// gift's value is credited (patient.balance.amount goes negative, i.e. the
// patient is now owed that much by the clinic). Single-use.
func RedeemGiftCode(c echo.Context) error {
	var req struct {
		Code       string `json:"code"`
		PatientID  string `json:"patientId"`
		CurrencyID string `json:"currencyId"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	user := c.Get("user").(store.User)
	gift, err := store.ApplyGiftByCode(req.Code, req.PatientID, req.CurrencyID, user.DisplayName)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "gift card not found"})
		}
		log.Println("Error: [RedeemGiftCode]:", err)
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: gift})
}

func UpdateDiscount(c echo.Context) error {
	var updates map[string]any
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")
	delete(updates, "currentUsages")

	d := store.Discount{ID: c.Param("id")}
	if err := d.Update(updates); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "discount not found"})
		}
		log.Println("Error: [UpdateDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to update discount"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: d})
}

func DeleteDiscount(c echo.Context) error {
	d := store.Discount{ID: c.Param("id")}
	if err := d.Delete(); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "discount not found"})
		}
		log.Println("Error: [DeleteDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to delete discount"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
