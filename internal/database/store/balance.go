package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"
)

type Balance struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entityType"`
	EntityID   *string `json:"entityId"`
	EntityName string  `json:"entityName"`
	CurrencyID string  `json:"currencyId"`
	Amount     float64 `json:"amount"`
	TotalIn    float64 `json:"totalIn"`
	TotalOut   float64 `json:"totalOut"`
	CreatedAt  Date    `json:"createdAt"`
	UpdatedAt  Date    `json:"updatedAt"`
	// Nested
	RecentTransactions BalanceTransactionList `json:"recentTransactions,omitempty"`
}

const balanceColumnsNoId = `entity_type, entity_id, entity_name, currency_id, amount, total_in, total_out, created_at, updated_at`
const balanceColumns = `id, ` + balanceColumnsNoId

type BalanceList []Balance

func (m *Balance) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Balance row")
	}
	return row.Scan(&m.ID, &m.EntityType, &m.EntityID, &m.EntityName, &m.CurrencyID, &m.Amount, &m.TotalIn, &m.TotalOut, &m.CreatedAt, &m.UpdatedAt)
}

func (l *BalanceList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Balance rows")
	}
	*l = BalanceList{}
	for rows.Next() {
		var item Balance
		err := rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.EntityName, &item.CurrencyID, &item.Amount, &item.TotalIn, &item.TotalOut, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			return err
		}
		*l = append(*l, item)
	}
	return nil
}

func (b *Balance) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(b.EntityType, "Entity type"); msg != "" {
		e["entityType"] = msg
	} else if msg := validation.OneOf(b.EntityType, []string{"patient", "employee", "self", "supplier", "expense"}, "Entity type"); msg != "" {
		e["entityType"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func (b *Balance) GetEntityType(id string, tx *sql.Tx) string {
	var entityType string
	tx.QueryRow(`SELECT entity_type FROM balances WHERE id = ?`, id).Scan(&entityType)
	return entityType
}

// Balance

func (b *Balance) GetOrCreate() error {
	if b.CurrencyID == "" {
		b.CurrencyID = USDCurrencyID
	}
	now := DateNow()
	entityID := ""
	if b.EntityID != nil {
		entityID = *b.EntityID
	}
	id := balanceID(b.EntityType, entityID, b.CurrencyID)
	// INSERT OR IGNORE avoids race conditions with the UNIQUE(entity_type, entity_id, currency_id) constraint
	_, err := DB.Exec(`INSERT OR IGNORE INTO balances (`+balanceColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, b.EntityType, b.EntityID, b.EntityName, b.CurrencyID, 0, 0, 0, now, now)
	if err != nil {
		return err
	}
	return b.ScanRow(RDB.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type = ? AND entity_id = ? AND currency_id = ?`,
		b.EntityType, b.EntityID, b.CurrencyID))
}

func (b *Balance) GetByID(id string) error {
	err := b.ScanRow(RDB.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE id = ?`, id))
	if err != nil {
		return err
	}
	b.RecentTransactions.GetByBalanceID(b.ID)
	return nil
}

func (b *Balance) GetByEntityID(entityType, entityID string) error {
	err := b.ScanRow(RDB.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type = ? AND entity_id = ? AND currency_id = ?`, entityType, entityID, USDCurrencyID))
	if err != nil {
		return err
	}
	return nil
}

// releaseEntityBalances deletes an entity's balances as part of deleting the
// entity, or refuses with ErrConflict while one is in use: it holds an amount,
// or a transaction or an invoice names it. A balance names its entity by type
// and id, without a foreign key, so every delete of an entity that can own a
// balance goes through here; what names the entity in the refusal.
func releaseEntityBalances(tx *sql.Tx, entityType, entityID, what string) error {
	var inUse int
	if err := tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM balances b
			WHERE b.entity_type = ? AND b.entity_id = ?
			AND (
				b.amount != 0
				OR EXISTS (
					SELECT 1 FROM balance_transactions bt
					WHERE bt.from_balance_id = b.id OR bt.to_balance_id = b.id
				)
				OR EXISTS (
					SELECT 1 FROM invoices i
					WHERE i.from_balance_id = b.id OR i.to_balance_id = b.id
				)
			)
		)`, entityType, entityID).Scan(&inUse); err != nil {
		return err
	}
	if inUse != 0 {
		return fmt.Errorf("%w: Can't delete %s while it's in use", ErrConflict, what)
	}
	_, err := tx.Exec(`DELETE FROM balances WHERE entity_type = ? AND entity_id = ?`, entityType, entityID)
	return err
}

func (b *BalanceList) GetAll(entityType string, params ListParams) (int, error) {
	where := " WHERE currency_id = ?"
	args := []any{USDCurrencyID}
	if entityType != "" {
		where += " AND entity_type = ?"
		args = append(args, entityType)
	}
	if fc, fa := params.FilterClause("entity_name"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM balances"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"entityType": "entity_type",
		"entityName": "entity_name",
		"currencyId": "currency_id",
		"amount":     "amount",
		"totalIn":    "total_in",
		"totalOut":   "total_out",
		"createdAt":  "created_at",
		"updatedAt":  "updated_at",
	}, "entity_type, entity_name")
	query := `SELECT ` + balanceColumns + ` FROM balances` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	err = b.ScanRows(rows)
	return total, err
}
