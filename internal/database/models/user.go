package models

import (
	"database/sql"
	"errors"
	"fmt"
)

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	DisplayName  string `json:"displayName"`
	Role         string `json:"role"`
	IsActive     bool   `json:"isActive"`
	CreatedAt    Date `json:"createdAt"`
	UpdatedAt    Date `json:"updatedAt"`
}

const userColumnsNoId = `username, password_hash, display_name, role, is_active, created_at, updated_at`
const userColumns = `id, ` + userColumnsNoId

type UserList []User

func (m *User) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil User row")
	}
	var isActiveRaw int
	err := row.Scan(&m.ID, &m.Username, &m.PasswordHash, &m.DisplayName, &m.Role, &isActiveRaw, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return err
	}
	m.IsActive = isActiveRaw == 1
	return nil
}

func (l *UserList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil User rows")
	}
	*l = UserList{}
	for rows.Next() {
		var item User
		var isActiveRaw int
		err := rows.Scan(&item.ID, &item.Username, &item.PasswordHash, &item.DisplayName, &item.Role, &isActiveRaw, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		item.IsActive = isActiveRaw == 1
		*l = append(*l, item)
	}
	return nil
}

func (u *User) GetByID(id string) error {
	err := u.ScanRow(DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if err != nil {
		return fmt.Errorf("get user by id: %w", err)
	}
	return nil
}

func (u *User) GetByUsername(username string) error {
	err := u.ScanRow(DB.QueryRow(`SELECT `+userColumns+` FROM users WHERE username = ?`, username))
	if err != nil {
		return fmt.Errorf("get user by username: %w", err)
	}
	return nil
}
