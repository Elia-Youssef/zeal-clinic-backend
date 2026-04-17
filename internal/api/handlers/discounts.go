package handlers

import (
	"clinic-api/internal/database/models"
	"clinic-api/internal/utils"
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

func GetAllDiscounts(c echo.Context) error {
	params := parseListParams(c)
	items := models.DiscountList{}
	total, err := items.GetAll(params)
	if err != nil {
		log.Println("Error: [GetAllDiscounts]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch discounts"})
	}
	if items == nil {
		items = []models.Discount{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetItemDiscounts(c echo.Context) error {
	items := models.DiscountList{}
	if err := items.GetByItem(c.Param("itemId")); err != nil {
		log.Println("Error: [GetItemDiscounts]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch discounts"})
	}
	if items == nil {
		items = []models.Discount{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: items})
}

func GetDiscountByID(c echo.Context) error {
	d := models.Discount{}
	if err := d.GetByID(c.Param("id")); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "discount not found"})
		}
		log.Println("Error: [GetDiscountByID]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch discount"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: d})
}

func CreateDiscount(c echo.Context) error {
	var d models.Discount
	if err := c.Bind(&d); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := d.IsValid(); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "validation failed"})
	}
	if err := d.Create(); err != nil {
		log.Println("Error: [CreateDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create discount"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: d})
}

func UpdateDiscount(c echo.Context) error {
	var updates map[string]interface{}
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	delete(updates, "id")
	delete(updates, "createdAt")
	delete(updates, "currentUsages")

	d := models.Discount{ID: c.Param("id")}
	if err := d.Update(updates); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "discount not found"})
		}
		log.Println("Error: [UpdateDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to update discount"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: d})
}

func DeleteDiscount(c echo.Context) error {
	d := models.Discount{ID: c.Param("id")}
	if err := d.Delete(); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "discount not found"})
		}
		log.Println("Error: [DeleteDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete discount"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// Discount Items

func AddDiscountItem(c echo.Context) error {
	var item models.DiscountItem
	if err := c.Bind(&item); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	if err := models.CreateDiscountItem(c.Param("id"), &item); err != nil {
		log.Println("Error: [AddDiscountItem]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to add discount item"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: item})
}

func RemoveDiscountItem(c echo.Context) error {
	if err := models.DeleteDiscountItem(c.Param("itemId")); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "discount item not found"})
		}
		log.Println("Error: [RemoveDiscountItem]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to remove discount item"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}

// Vouchers

func GetAllVouchers(c echo.Context) error {
	params := parseListParams(c)
	items := models.VoucherList{}
	total, err := items.GetAll(params, "")
	if err != nil {
		log.Println("Error: [GetAllVouchers]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch vouchers"})
	}
	if items == nil {
		items = models.VoucherList{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetItemVouchers(c echo.Context) error {
	params := parseListParams(c)
	items := models.VoucherList{}
	total, err := items.GetAll(params, c.Param("itemId"))
	if err != nil {
		log.Println("Error: [GetItemVouchers]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch vouchers"})
	}
	if items == nil {
		items = models.VoucherList{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: utils.PaginatedList{Items: items, Total: total}})
}

func GetVouchersByDiscount(c echo.Context) error {
	var vouchers models.VoucherList
	if err := vouchers.GetByDiscount(c.Param("id")); err != nil {
		log.Println("Error: [GetVouchersByDiscount]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch vouchers"})
	}
	if vouchers == nil {
		vouchers = models.VoucherList{}
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: vouchers})
}

func GetVoucherByCode(c echo.Context) error {
	v := models.Voucher{}
	if err := v.GetByCode(c.Param("code")); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "voucher not found"})
		}
		log.Println("Error: [GetVoucherByCode]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch voucher"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: v})
}

func CreateVoucher(c echo.Context) error {
	var v models.Voucher
	if err := c.Bind(&v); err != nil {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "invalid request"})
	}
	v.DiscountID = c.Param("id")
	if v.Code == "" {
		return c.JSON(http.StatusBadRequest, utils.Response{Error: "code is required"})
	}
	if err := v.Create(); err != nil {
		log.Println("Error: [CreateVoucher]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to create voucher"})
	}
	return c.JSON(http.StatusCreated, utils.Response{Success: true, Data: v})
}

func MarkVoucherUsed(c echo.Context) error {
	v := models.Voucher{}
	if err := v.GetByID(c.Param("voucherId")); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "voucher not found"})
		}
		log.Println("Error: [MarkVoucherUsed]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to fetch voucher"})
	}
	if err := v.MarkUsed(); err != nil {
		log.Println("Error: [MarkVoucherUsed]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to mark voucher as used"})
	}
	v.IsUsed = 1
	return c.JSON(http.StatusOK, utils.Response{Success: true, Data: v})
}

func DeleteVoucher(c echo.Context) error {
	if err := models.DeleteVoucher(c.Param("voucherId")); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, utils.Response{Error: "voucher not found"})
		}
		log.Println("Error: [DeleteVoucher]:", err)
		return c.JSON(http.StatusInternalServerError, utils.Response{Error: "failed to delete voucher"})
	}
	return c.JSON(http.StatusOK, utils.Response{Success: true})
}
