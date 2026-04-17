package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetInvoiceItemDiscounts(c echo.Context) error {
	items := models.InvoiceItemDiscountList{}
	if err := items.GetByInvoiceItem(c.Param("itemId")); err != nil {
		log.Println("Error: [GetInvoiceItemDiscounts]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch invoice item discounts"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func ApplyInvoiceItemDiscount(c echo.Context) error {
	var iid models.InvoiceItemDiscount
	if err := c.Bind(&iid); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	iid.InvoiceItemID = c.Param("itemId")

	if err := iid.Create(); err != nil {
		log.Println("Error: [ApplyInvoiceItemDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to apply discount"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: iid})
}

func RemoveInvoiceItemDiscount(c echo.Context) error {
	if err := models.DeleteInvoiceItemDiscount(c.Param("discountId")); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "invoice item discount not found"})
		}
		log.Println("Error: [RemoveInvoiceItemDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove discount"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
