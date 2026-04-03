package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

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
	var args []interface{}
	if fc, fa := params.FilterClause("code", "name", "symbol"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM currencies"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + currencyColumns + ` FROM currencies` + where + ` ORDER BY code` + params.PaginationClause()
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
	var args []interface{}
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

func (cur *Currency) Create() error {
	cur.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	cur.CreatedAt = now
	cur.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO currencies (`+currencyColumns+`) VALUES (?,?,?,?,?,?,?)`,
		cur.ID, cur.Code, cur.Name, cur.Symbol, cur.ExchangeRate, cur.CreatedAt, cur.UpdatedAt)
	return err
}

func (cur *Currency) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"code": "code", "name": "name", "symbol": "symbol", "exchangeRate": "exchange_rate",
	}
	setClauses := ""
	var args []interface{}
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
		return err
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
		return sql.ErrNoRows
	}
	return nil
}
