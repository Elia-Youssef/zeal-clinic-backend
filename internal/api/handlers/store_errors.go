package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"clinic-api/internal/tracking"
	"clinic-api/internal/validation"

	"github.com/labstack/echo/v4"
)

// storeError answers the error of a store call in the response envelope. The
// store's error kinds map to their status with the store's own user-facing
// message (kindMessage); a field-level validation map answers the generic
// input message, because its text is field names, not prose. notFound names
// the addressed record for a 404, which the store's own not-found error
// cannot do. A conflict is worth a warning in the tracking log (a double
// booking or a lost race is operational signal, not noise). Anything else is
// logged and answered as a 500 with fallback.
func storeError(c echo.Context, err error, notFound, fallback string) error {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return c.JSON(http.StatusNotFound, httpx.Response{Error: notFound})
	case errors.Is(err, store.ErrConflict):
		tracking.Warn(c, err.Error())
		return c.JSON(http.StatusConflict, httpx.Response{Error: kindMessage(err, store.ErrConflict)})
	case errors.Is(err, store.ErrValidation):
		var fieldErrs validation.Errors
		if errors.As(err, &fieldErrs) {
			return c.JSON(http.StatusBadRequest, httpx.Response{Error: "Please check your input"})
		}
		return c.JSON(http.StatusBadRequest, httpx.Response{Error: kindMessage(err, store.ErrValidation)})
	default:
		log.Printf("Error: %s: %v", fallback, err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: fallback})
	}
}

// kindMessage is the user-facing text a store error carries after its kind
// ("conflict: This code is already in use"). Context the store wrapped around
// it on the way out ("create gift discount: ...") is for the logs, not the
// client.
func kindMessage(err, kind error) string {
	msg := err.Error()
	if _, text, ok := strings.Cut(msg, kind.Error()+": "); ok {
		return text
	}
	return msg
}
