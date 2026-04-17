package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Balance struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entityType"`
	EntityID   *string `json:"entityId"`
	EntityName string  `json:"entityName"`
	CurrencyID string  `json:"currencyId"`
	Amount     float64 `json:"amount"`
	CreatedAt  Date    `json:"createdAt"`
	UpdatedAt  Date    `json:"updatedAt"`
	// Nested
	RecentTransactions BalanceTransactionList `json:"recentTransactions,omitempty"`
}

const balanceColumnsNoId = `entity_type, entity_id, entity_name, currency_id, amount, created_at, updated_at`
const balanceColumns = `id, ` + balanceColumnsNoId

type BalanceList []Balance

func (m *Balance) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Balance row")
	}
	return row.Scan(&m.ID, &m.EntityType, &m.EntityID, &m.EntityName, &m.CurrencyID, &m.Amount, &m.CreatedAt, &m.UpdatedAt)
}

func (l *BalanceList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Balance rows")
	}
	*l = BalanceList{}
	for rows.Next() {
		var item Balance
		err := rows.Scan(&item.ID, &item.EntityType, &item.EntityID, &item.EntityName, &item.CurrencyID, &item.Amount, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (b *Balance) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(b.EntityType, "Entity type"); msg != "" {
		e["entityType"] = msg
	} else if msg := validation.OneOf(b.EntityType, []string{"patient", "employee", "self", "supplier"}, "Entity type"); msg != "" {
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
	now := DateNow()
	id := uuid.Must(uuid.NewV7()).String()
	// INSERT OR IGNORE avoids race conditions with the UNIQUE(entity_type, entity_id, currency_id) constraint
	_, err := DB.Exec(`INSERT OR IGNORE INTO balances (`+balanceColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
		id, b.EntityType, b.EntityID, b.EntityName, b.CurrencyID, 0, now, now)
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
	err := b.ScanRow(RDB.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type = ? AND entity_id = ? ORDER BY currency_id`, entityType, entityID))
	if err != nil {
		return err
	}
	return nil
}

func (b *BalanceList) GetAll(entityType string, params ListParams) (int, error) {
	where := " WHERE 1=1"
	var args []interface{}
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

	query := `SELECT ` + balanceColumns + ` FROM balances` + where + ` ORDER BY entity_type, entity_name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	err = b.ScanRows(rows)
	return total, err
}
