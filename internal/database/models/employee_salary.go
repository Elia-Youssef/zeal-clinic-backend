package models

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
	if msg := validation.Required(s.CurrencyID, "Currency ID"); msg != "" {
		e["currencyId"] = msg
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

func (s *EmployeeSalary) GetByEmployee(employeeID string) ([]EmployeeSalary, error) {
	rows, err := DB.Query(`SELECT `+employeeSalaryColumns+` FROM employee_salaries WHERE employee_id = ? ORDER BY is_active DESC, effective_date DESC`, employeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list EmployeeSalaryList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
}

func (s *EmployeeSalary) GetByID(id string) error {
	return s.ScanRow(DB.QueryRow(`SELECT `+employeeSalaryColumns+` FROM employee_salaries WHERE id = ?`, id))
}

func (s *EmployeeSalary) Create() error {
	s.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	s.CreatedAt = now
	s.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO employee_salaries (`+employeeSalaryColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		s.ID, s.EmployeeID, s.Amount, s.CurrencyID, BoolToInt(s.IsActive), s.EffectiveDate, s.Notes, s.CreatedAt, s.UpdatedAt)
	return err
}

func (s *EmployeeSalary) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"amount": "amount", "currencyId": "currency_id", "isActive": "is_active",
		"effectiveDate": "effective_date", "notes": "notes",
	}
	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if dbCol == "is_active" {
				if b, ok := val.(bool); ok {
					val = BoolToInt(b)
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
		return err
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
		return sql.ErrNoRows
	}
	return nil
}
