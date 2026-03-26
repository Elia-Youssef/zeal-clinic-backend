package models

import (
	"database/sql"
	"fmt"
	"time"
)

type TeamMember struct {
	ID             string      `json:"id"`
	UserID         string      `json:"userId"`
	FirstName      string      `json:"firstName"`
	LastName       string      `json:"lastName"`
	Role           string      `json:"role"`
	Contact        string      `json:"contact"`
	Email          string      `json:"email"`
	DateOfBirth    string      `json:"dateOfBirth"`
	EmploymentType string      `json:"employmentType"`
	Salary         float64     `json:"salary"`
	Schedule       StringSlice `json:"schedule"`
	OffDays        StringSlice `json:"offDays"`
	HireDate       string      `json:"hireDate"`
	Status         string      `json:"status"`
	CreatedAt      string      `json:"createdAt"`
	UpdatedAt      string      `json:"updatedAt"`
}

func (m *TeamMember) GetAll() ([]TeamMember, error) {
	rows, err := DB.Query(`SELECT id, user_id, first_name, last_name, role, contact, email, date_of_birth,
		employment_type, salary, schedule, off_days, hire_date, status, created_at, updated_at
		FROM team_members ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.ID, &m.UserID, &m.FirstName, &m.LastName, &m.Role, &m.Contact, &m.Email, &m.DateOfBirth,
			&m.EmploymentType, &m.Salary, &m.Schedule, &m.OffDays, &m.HireDate, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (m *TeamMember) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, user_id, first_name, last_name, role, contact, email, date_of_birth,
		employment_type, salary, schedule, off_days, hire_date, status, created_at, updated_at
		FROM team_members WHERE id = ?`, id).
		Scan(&m.ID, &m.UserID, &m.FirstName, &m.LastName, &m.Role, &m.Contact, &m.Email, &m.DateOfBirth,
			&m.EmploymentType, &m.Salary, &m.Schedule, &m.OffDays, &m.HireDate, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	return err
}

func (m *TeamMember) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var nextNum int
	err = tx.QueryRow(`UPDATE counters SET value = value + 1 WHERE name = 'team_member' RETURNING value`).Scan(&nextNum)
	if err != nil {
		return fmt.Errorf("counter: %w", err)
	}

	m.ID = fmt.Sprintf("TM-%03d", nextNum)
	now := time.Now().Format(time.RFC3339)
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err = tx.Exec(`INSERT INTO team_members (id, user_id, first_name, last_name, role, contact, email, date_of_birth,
		employment_type, salary, schedule, off_days, hire_date, status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.UserID, m.FirstName, m.LastName, m.Role, m.Contact, m.Email, m.DateOfBirth,
		m.EmploymentType, m.Salary, m.Schedule, m.OffDays, m.HireDate, m.Status, m.CreatedAt, m.UpdatedAt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (m *TeamMember) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"userId": "user_id", "firstName": "first_name", "lastName": "last_name", "role": "role",
		"contact": "contact", "email": "email", "dateOfBirth": "date_of_birth",
		"employmentType": "employment_type", "salary": "salary",
		"schedule": "schedule", "offDays": "off_days",
		"hireDate": "hire_date", "status": "status",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			switch dbCol {
			case "schedule", "off_days":
				if slice, ok := val.([]interface{}); ok {
					s := StringSlice{}
					for _, v := range slice {
						if str, ok := v.(string); ok {
							s = append(s, str)
						}
					}
					v, _ := s.Value()
					val = v
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
		return m.GetByID(m.ID)
	}

	setClauses += ", updated_at = ?"
	args = append(args, time.Now().Format(time.RFC3339))
	args = append(args, m.ID)
	_, err := DB.Exec("UPDATE team_members SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return m.GetByID(m.ID)
}

func (m *TeamMember) Delete() error {
	res, err := DB.Exec("DELETE FROM team_members WHERE id = ?", m.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
