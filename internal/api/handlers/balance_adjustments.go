package handlers

import (
	"clinic-api/internal/api/httpx"
	"clinic-api/internal/database/store"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// DeleteBalanceTransaction removes a balance transaction (payment, adjustment,
// write-off, etc.) and reverses the balance updates it made. If the transaction
// was created as part of a paired charge+payment (expense payment), both legs
// are removed atomically.
func DeleteBalanceTransaction(c echo.Context) error {
	bt := store.BalanceTransaction{ID: c.Param("id")}
	if err := bt.Delete(); errors.Is(err, store.ErrNotFound) {
		return c.JSON(http.StatusNotFound, httpx.Response{Error: "Transaction not found"})
	} else if err != nil {
		log.Println("Error: DeleteBalanceTransaction:", err)
		return c.JSON(http.StatusInternalServerError, httpx.Response{Error: "Couldn't delete transaction"})
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

func resolveEntityName(entityType, entityID string) (string, error) {
	switch entityType {
	case "patient":
		var p store.Patient
		if err := p.GetByID(entityID); err != nil {
			return "", errors.New("patient not found")
		}
		return p.FirstName + " " + p.LastName, nil
	case "supplier":
		var s store.Supplier
		if err := s.GetByID(entityID); err != nil {
			return "", errors.New("supplier not found")
		}
		return s.Name, nil
	case "employee":
		var e store.Employee
		if err := e.GetByID(entityID); err != nil {
			return "", errors.New("employee not found")
		}
		return e.FirstName + " " + e.LastName, nil
	case "expense":
		var ex store.Expense
		if err := ex.GetByID(entityID); err != nil {
			return "", errors.New("expense not found")
		}
		return ex.Name, nil
	}
	return "", fmt.Errorf("unsupported entity type: %s", entityType)
}
