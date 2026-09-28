package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
)

// Currencies use their ISO code as the primary key: it's a stable, unique
// natural key that's identical across synced DBs, so a separate UUID adds no
// sync value and only obscures which row is which.
const USDCurrencyID = "USD"

type Currency struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Symbol       string  `json:"symbol"`
	ExchangeRate float64 `json:"exchangeRate"`
	CreatedAt    Date    `json:"createdAt"`
	UpdatedAt    Date    `json:"updatedAt"`
}

func (c *Currency) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(c.Code, "Code"); msg != "" {
		e["code"] = msg
	}
	if msg := validation.Required(c.Name, "Name"); msg != "" {
		e["name"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const currencyColumnsNoId = `code, name, symbol, exchange_rate, created_at, updated_at`
const currencyColumns = `id, ` + currencyColumnsNoId

type CurrencyList []Currency

func (m *Currency) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Currency row")
	}
	return row.Scan(&m.ID, &m.Code, &m.Name, &m.Symbol, &m.ExchangeRate, &m.CreatedAt, &m.UpdatedAt)
}

func (l *CurrencyList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Currency rows")
	}
	*l = CurrencyList{}
	for rows.Next() {
		var item Currency
		err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.Symbol, &item.ExchangeRate, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (cur *CurrencyList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("code", "name", "symbol"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM currencies"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"code":         "code",
		"name":         "name",
		"symbol":       "symbol",
		"exchangeRate": "exchange_rate",
		"createdAt":    "created_at",
		"updatedAt":    "updated_at",
	}, "code")
	query := `SELECT ` + currencyColumns + ` FROM currencies` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	err = cur.ScanRows(rows)
	return total, err
}

func GetCurrencyDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM currencies` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DropdownItem
	for rows.Next() {
		var item DropdownItem
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (cur *Currency) GetByID(id string) error {
	return cur.ScanRow(RDB.QueryRow(`SELECT `+currencyColumns+` FROM currencies WHERE id = ?`, id))
}

func (cur *Currency) GetByCode(code string) error {
	return cur.ScanRow(RDB.QueryRow(`SELECT `+currencyColumns+` FROM currencies WHERE code = ?`, code))
}

// LBPRate returns the current LBP-per-USD exchange rate from the currencies
// table, or 0 if it can't be read so callers can skip the conversion.
func LBPRate() float64 {
	if RDB == nil {
		return 0
	}
	var c Currency
	if err := c.GetByCode("LBP"); err != nil {
		return 0
	}
	return c.ExchangeRate
}

func (cur *Currency) Create() error {
	cur.ID = cur.Code
	now := DateNow()
	cur.CreatedAt = now
	cur.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO currencies (`+currencyColumns+`) VALUES (?,?,?,?,?,?,?)`,
		cur.ID, cur.Code, cur.Name, cur.Symbol, cur.ExchangeRate, cur.CreatedAt, cur.UpdatedAt)
	return constraintError(err, "A currency with this code already exists")
}

func (cur *Currency) Update(updates map[string]any) error {
	cols := map[string]string{
		"name": "name", "symbol": "symbol", "exchangeRate": "exchange_rate",
	}
	setClauses := ""
	var args []any
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			args = append(args, val)
		}
	}
	if setClauses == "" {
		return cur.GetByID(cur.ID)
	}
	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, cur.ID)
	if _, err := DB.Exec("UPDATE currencies SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return constraintError(err, "A currency with this code already exists")
	}
	return cur.GetByID(cur.ID)
}

func (cur *Currency) Delete() error {
	res, err := DB.Exec("DELETE FROM currencies WHERE id = ?", cur.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
