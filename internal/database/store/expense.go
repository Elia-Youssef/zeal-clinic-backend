package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Expense struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Notes     string `json:"notes"`
	CreatedAt Date   `json:"createdAt"`
	UpdatedAt Date   `json:"updatedAt"`
}

func (e *Expense) IsValid() error {
	errs := make(validation.Errors)
	if msg := validation.Required(e.Name, "Name"); msg != "" {
		errs["name"] = msg
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

const expenseColumnsNoId = `name, notes, created_at, updated_at`
const expenseColumns = `id, ` + expenseColumnsNoId

type ExpenseList []Expense

func (e *Expense) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Expense row")
	}
	return row.Scan(&e.ID, &e.Name, &e.Notes, &e.CreatedAt, &e.UpdatedAt)
}

func (l *ExpenseList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Expense rows")
	}
	*l = ExpenseList{}
	for rows.Next() {
		var item Expense
		err := rows.Scan(&item.ID, &item.Name, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (l *ExpenseList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name", "notes"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM expenses"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"name":      "name",
		"notes":     "notes",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
	}, "name")
	query := `SELECT ` + expenseColumns + ` FROM expenses` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := l.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

func GetExpenseDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM expenses` + where + ` ORDER BY name` + params.PaginationClause()
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

func (e *Expense) GetByID(id string) error {
	err := e.ScanRow(RDB.QueryRow(`SELECT `+expenseColumns+` FROM expenses WHERE id = ?`, id))
	if err != nil {
		return err
	}
	return nil
}

func (e *Expense) Create() error {
	e.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	e.CreatedAt = now
	e.UpdatedAt = now

	_, err := DB.Exec(`INSERT INTO expenses (`+expenseColumns+`) VALUES (?,?,?,?,?)`,
		e.ID, e.Name, e.Notes, e.CreatedAt, e.UpdatedAt)
	if err != nil {
		return err
	}

	// Create a balance for the expense in each currency
	var currencies CurrencyList
	if _, err := currencies.GetAll(ListParams{}); err == nil {
		for _, cur := range currencies {
			bal := Balance{
				EntityType: "expense",
				EntityID:   &e.ID,
				EntityName: e.Name,
				CurrencyID: cur.ID,
			}
			bal.GetOrCreate()
		}
	}

	return nil
}

func (e *Expense) Update(updates map[string]any) error {
	cols := map[string]string{
		"name": "name", "notes": "notes",
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
		return e.GetByID(e.ID)
	}

	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, e.ID)
	if _, err := DB.Exec("UPDATE expenses SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}

	return e.GetByID(e.ID)
}

func (e *Expense) Delete() error {
	res, err := DB.Exec("DELETE FROM expenses WHERE id = ?", e.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
