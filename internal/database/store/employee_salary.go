package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (s *EmployeeSalary) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Positive(s.Amount, "Amount"); msg != "" {
		e["amount"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

type EmployeeSalary struct {
	ID            string  `json:"id"`
	EmployeeID    string  `json:"employeeId"`
	Amount        float64 `json:"amount"`
	CurrencyID    string  `json:"currencyId"`
	IsActive      bool    `json:"isActive"`
	EffectiveDate Date    `json:"effectiveDate"`
	Notes         string  `json:"notes"`
	CreatedAt     Date    `json:"createdAt"`
	UpdatedAt     Date    `json:"updatedAt"`
}

const employeeSalaryColumnsNoId = `employee_id, amount, currency_id, is_active, effective_date, notes, created_at, updated_at`
const employeeSalaryColumns = `id, ` + employeeSalaryColumnsNoId

type EmployeeSalaryList []EmployeeSalary

func (m *EmployeeSalary) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil EmployeeSalary row")
	}
	var isActive int
	err := row.Scan(&m.ID, &m.EmployeeID, &m.Amount, &m.CurrencyID, &isActive,
		&m.EffectiveDate, &m.Notes, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return err
	}
	m.IsActive = isActive == 1
	return nil
}

func (l *EmployeeSalaryList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil EmployeeSalary rows")
	}
	*l = EmployeeSalaryList{}
	for rows.Next() {
		var item EmployeeSalary
		var isActive int
		err := rows.Scan(&item.ID, &item.EmployeeID, &item.Amount, &item.CurrencyID, &isActive,
			&item.EffectiveDate, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		item.IsActive = isActive == 1
		*l = append(*l, item)
	}
	return nil
}

func (s *EmployeeSalaryList) GetByEmployee(employeeID string, params ListParams) (int, error) {
	var total int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM employee_salaries WHERE employee_id = ?`, employeeID).Scan(&total); err != nil {
		return 0, err
	}

	rows, err := RDB.Query(`SELECT `+employeeSalaryColumns+` FROM employee_salaries WHERE employee_id = ? ORDER BY is_active DESC, effective_date DESC`+params.PaginationClause(), employeeID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	return total, s.ScanRows(rows)
}

func (s *EmployeeSalary) GetByID(id string) error {
	return s.ScanRow(RDB.QueryRow(`SELECT `+employeeSalaryColumns+` FROM employee_salaries WHERE id = ?`, id))
}

func (s *EmployeeSalary) Create() error {
	return constraintError(s.create(), "")
}

func (s *EmployeeSalary) create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	s.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	s.CreatedAt = now
	s.UpdatedAt = now
	s.IsActive = true
	s.CurrencyID = USDCurrencyID
	s.Amount = Round2(s.Amount)

	// Deactivate existing salaries for this employee
	if _, err := tx.Exec(`UPDATE employee_salaries SET is_active = 0, updated_at = ? WHERE employee_id = ? AND is_active = 1`,
		now, s.EmployeeID); err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO employee_salaries (`+employeeSalaryColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		s.ID, s.EmployeeID, s.Amount, s.CurrencyID, BoolToInt(s.IsActive), s.EffectiveDate, s.Notes, s.CreatedAt, s.UpdatedAt); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *EmployeeSalary) Update(updates map[string]any) error {
	cols := map[string]string{
		"amount": "amount", "isActive": "is_active",
		"effectiveDate": "effective_date", "notes": "notes",
	}
	setClauses := ""
	var args []any
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if dbCol == "is_active" {
				if b, ok := val.(bool); ok {
					val = BoolToInt(b)
				}
			}
			if dbCol == "amount" {
				if f, ok := val.(float64); ok {
					val = Round2(f)
				}
			}
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			args = append(args, val)
		}
	}
	if setClauses == "" {
		return s.GetByID(s.ID)
	}
	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, s.ID)
	_, err := DB.Exec("UPDATE employee_salaries SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return constraintError(err, "")
	}
	return s.GetByID(s.ID)
}

func (s *EmployeeSalary) Delete() error {
	res, err := DB.Exec("DELETE FROM employee_salaries WHERE id = ?", s.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
