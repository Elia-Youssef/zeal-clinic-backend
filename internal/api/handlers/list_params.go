package handlers

import (
	"clinic-api/internal/database/models"
	"strconv"

	"github.com/labstack/echo/v4"
)

func parseListParams(c echo.Context) models.ListParams {
	var params models.ListParams
	if v := c.QueryParam("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			params.Offset = n
		}
	}
	if v := c.QueryParam("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			params.Limit = n
		}
	}
	params.Filter = c.QueryParam("filter")
	return params
}
