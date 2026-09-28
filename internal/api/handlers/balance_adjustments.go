package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

func DeleteClientPayment(c echo.Context) error {
	return deleteBalanceTransaction(c, "patient")
}

func DeleteSupplierPayment(c echo.Context) error {
	return deleteBalanceTransaction(c, "supplier")
}

func DeleteEmployeePayment(c echo.Context) error {
	return deleteBalanceTransaction(c, "employee")
}

func DeleteExpensePayment(c echo.Context) error {
	return deleteBalanceTransaction(c, "expense")
}

// deleteBalanceTransaction removes a payment-list transaction only when it
// belongs to the endpoint's entity scope. Expense payment pairs are still
// removed atomically by the store.
func deleteBalanceTransaction(c echo.Context, entityType string) error {
	bt := store.BalanceTransaction{ID: c.Param("id")}
	if err := bt.DeleteForEntityType(entityType); err != nil {
		return storeError(c, err, "Transaction not found", "Couldn't delete transaction")
	}
	return c.JSON(http.StatusOK, httpx.Response{Success: true})
}

// createEntityBalanceCorrection records an adjustment or write-off transaction
// between an entity's balance and the clinic's self balance. It resolves both
// balances by entity ID and direction:
//   - "incoming": entity balance (FROM) to self balance (TO)
//   - "outgoing": self balance (FROM) to entity balance (TO)
//
// transactionType is "adjustment" or "write-off".
func createEntityBalanceCorrection(
	c echo.Context,
	entityType, entityID, currencyID string,
	amount float64,
	direction, transactionType, transactionMethod, description string,
) (*store.BalanceTransaction, error) {
	if currencyID == "" {
		currencyID = store.USDCurrencyID
	}

	entityName, err := resolveEntityName(entityType, entityID)
	if err != nil {
		return nil, err
	}

	entityBalance := store.Balance{
		EntityType: entityType,
		EntityID:   &entityID,
		EntityName: entityName,
		CurrencyID: currencyID,
	}
	if err := entityBalance.GetOrCreate(); err != nil {
		return nil, fmt.Errorf("resolve entity balance: %w", err)
	}

	selfID := "self"
	selfBalance := store.Balance{
		EntityType: "self",
		EntityID:   &selfID,
		EntityName: "Clinic",
		CurrencyID: currencyID,
	}
	if err := selfBalance.GetOrCreate(); err != nil {
		return nil, fmt.Errorf("resolve self balance: %w", err)
	}

	from, to := entityBalance, selfBalance
	if direction == "outgoing" {
		from, to = selfBalance, entityBalance
	}

	user := c.Get("user").(store.User)
	bt := store.BalanceTransaction{
		FromBalanceID:     from.ID,
		ToBalanceID:       to.ID,
		Amount:            amount,
		CurrencyID:        currencyID,
		TransactionType:   transactionType,
		TransactionMethod: transactionMethod,
		Description:       description,
		CreatedBy:         user.DisplayName,
	}
	if err := bt.Create(); err != nil {
		return nil, err
	}

	bt.FromEntityName = from.EntityName
	bt.ToEntityName = to.EntityName
	return &bt, nil
}

// resolveEntityName loads the display name of the entity an adjustment or
// write-off targets. A missing record answers the store's validation error
// (a 400 "<Entity> not found"), the way the payment handlers' explicit checks
// already do; any other error passes through.
func resolveEntityName(entityType, entityID string) (string, error) {
	notFound := func(entity string) error {
		return fmt.Errorf("%w: %s not found", store.ErrValidation, entity)
	}
	switch entityType {
	case "patient":
		var p store.Patient
		if err := p.GetByID(entityID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return "", notFound("Patient")
			}
			return "", err
		}
		return p.FirstName + " " + p.LastName, nil
	case "supplier":
		var s store.Supplier
		if err := s.GetByID(entityID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return "", notFound("Supplier")
			}
			return "", err
		}
		return s.Name, nil
	case "employee":
		var e store.Employee
		if err := e.GetByID(entityID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return "", notFound("Employee")
			}
			return "", err
		}
		return e.FirstName + " " + e.LastName, nil
	case "expense":
		var ex store.Expense
		if err := ex.GetByID(entityID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return "", notFound("Expense")
			}
			return "", err
		}
		return ex.Name, nil
	}
	return "", fmt.Errorf("unsupported entity type: %s", entityType)
}
