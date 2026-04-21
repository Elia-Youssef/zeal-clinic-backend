package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetInvoiceItemDiscounts(c echo.Context) error {
	items := store.InvoiceItemDiscountList{}
	if err := items.GetByInvoiceItem(c.Param("itemId")); err != nil {
		log.Println("Error: [GetInvoiceItemDiscounts]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to fetch invoice item discounts"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true, Data: items})
}

func ApplyInvoiceItemDiscount(c echo.Context) error {
	var iid store.InvoiceItemDiscount
	if err := c.Bind(&iid); err != nil {
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: "invalid request"})
	}
	iid.InvoiceItemID = c.Param("itemId")

	if err := iid.Create(); err != nil {
		log.Println("Error: [ApplyInvoiceItemDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to apply discount"})
	}
	return c.JSON(http.StatusCreated, httpx.Response{Success: true, Data: iid})
}

func RemoveInvoiceItemDiscount(c echo.Context) error {
	if err := store.DeleteInvoiceItemDiscount(c.Param("discountId")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return c.JSON(http.StatusNotFound, httpx.Response{Error: "invoice item discount not found"})
		}
		log.Println("Error: [RemoveInvoiceItemDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "failed to remove discount"})
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}
