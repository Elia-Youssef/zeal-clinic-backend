package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Employee struct {
	ID             string  `json:"id"`
	UserID         *string `json:"userId"`
	FirstName      string  `json:"firstName"`
	LastName       string  `json:"lastName"`
	Role           string `json:"role"`
	Contact        string `json:"contact"`
	Email          string `json:"email"`
	DateOfBirth    Date   `json:"dateOfBirth"`
	EmploymentType string `json:"employmentType"`
	CreatedAt      Date   `json:"createdAt"`
	UpdatedAt      Date   `json:"updatedAt"`
	// Nested
	Salaries      EmployeeSalaryList `json:"salaries,omitempty"`
	User          *User              `json:"user,omitempty"`
	Balance       []Balance          `json:"balance,omitempty"`
	LatestActions []AuditLogEntry    `json:"latestActions,omitempty"`
}

func (m *Employee) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(m.FirstName, "First name"); msg != "" {
		e["firstName"] = msg
	}
	if msg := validation.Required(m.LastName, "Last name"); msg != "" {
		e["lastName"] = msg
	}
	if msg := validation.Required(m.Role, "Role"); msg != "" {
		e["role"] = msg
	}
	if msg := validation.Required(m.Contact, "Contact"); msg != "" {
		e["contact"] = msg
	} else if msg := validation.Phone(m.Contact); msg != "" {
		e["contact"] = msg
	}
	if msg := validation.Email(m.Email); msg != "" {
		e["email"] = msg
	}
	if msg := validation.Required(m.EmploymentType, "Employment type"); msg != "" {
		e["employmentType"] = msg
	} else if msg := validation.OneOf(m.EmploymentType, []string{"Full-time", "Part-time"}, "Employment type"); msg != "" {
		e["employmentType"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const employeeColumnsNoId = `user_id, first_name, last_name, role, contact, email, date_of_birth, employment_type, created_at, updated_at`
const employeeColumns = `id, ` + employeeColumnsNoId

type EmployeeList []Employee

func (m *Employee) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Employee row")
	}
	return row.Scan(&m.ID, &m.UserID, &m.FirstName, &m.LastName, &m.Role, &m.Contact, &m.Email, &m.DateOfBirth,
		&m.EmploymentType, &m.CreatedAt, &m.UpdatedAt)
}

func (l *EmployeeList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Employee rows")
	}
	*l = EmployeeList{}
	for rows.Next() {
		var item Employee
		err := rows.Scan(&item.ID, &item.UserID, &item.FirstName, &item.LastName, &item.Role, &item.Contact, &item.Email, &item.DateOfBirth,
			&item.EmploymentType, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (m *EmployeeList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("first_name", "last_name", "role", "contact", "email"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM employees"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + employeeColumns + ` FROM employees` + where + ` ORDER BY id` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}

	if err := m.ScanRows(rows); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for i := range *m {
		(*m)[i].Salaries.GetByEmployee((*m)[i].ID)
	}
	return total, nil
}

func GetEmployeeDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("first_name", "last_name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, first_name || ' ' || last_name as name FROM employees` + where + ` ORDER BY first_name, last_name` + params.PaginationClause()
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

func (m *Employee) GetByID(id string) error {
	err := m.ScanRow(RDB.QueryRow(`SELECT `+employeeColumns+` FROM employees WHERE id = ?`, id))
	if err != nil {
		return err
	}
	m.Salaries.GetByEmployee(m.ID)
	// Load user
	if m.UserID != nil && *m.UserID != "" {
		var user User
		if err := user.GetByID(*m.UserID); err == nil {
			m.User = &user
		}
	}
	// Load balance
	m.Balance, _ = (&Balance{}).GetByEntityID("employee", m.ID)
	// Load latest actions (by user)
	if m.User != nil {
		m.LatestActions, _ = (&AuditLogEntry{}).GetByUserName(m.User.Username, 10)
	}
	return nil
}

func (m *Employee) Create() error {
	m.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err := DB.Exec(`INSERT INTO employees (`+employeeColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.UserID, m.FirstName, m.LastName, m.Role, m.Contact, m.Email, m.DateOfBirth,
		m.EmploymentType, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return err
	}

	// Create a balance for the employee in each currency
	var currencies CurrencyList
	if _, err := currencies.GetAll(ListParams{}); err == nil {
		entityName := m.FirstName + " " + m.LastName
		for _, cur := range currencies {
			bal := Balance{
				EntityType: "employee",
				EntityID:   &m.ID,
				EntityName: entityName,
				CurrencyID: cur.ID,
			}
			bal.GetOrCreate()
		}
	}

	return nil
}

func (m *Employee) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"userId": "user_id", "firstName": "first_name", "lastName": "last_name", "role": "role",
		"contact": "contact", "email": "email", "dateOfBirth": "date_of_birth",
		"employmentType": "employment_type",
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
		return m.GetByID(m.ID)
	}

	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, m.ID)
	_, err := DB.Exec("UPDATE employees SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return m.GetByID(m.ID)
}

func (m *Employee) Delete() error {
	res, err := DB.Exec("DELETE FROM employees WHERE id = ?", m.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
