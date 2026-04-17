package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
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
	err := u.ScanRow(RDB.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if err != nil {
		return fmt.Errorf("get user by id: %w", err)
	}
	return nil
}

func (u *User) GetByUsername(username string) error {
	err := u.ScanRow(RDB.QueryRow(`SELECT `+userColumns+` FROM users WHERE username = ?`, username))
	if err != nil {
		return fmt.Errorf("get user by username: %w", err)
	}
	return nil
}

func (u *User) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(u.Username, "Username"); msg != "" {
		e["username"] = msg
	} else if msg := validation.MinLength(u.Username, 3, "Username"); msg != "" {
		e["username"] = msg
	}
	if msg := validation.Required(u.DisplayName, "Display name"); msg != "" {
		e["displayName"] = msg
	}
	if msg := validation.Required(u.Role, "Role"); msg != "" {
		e["role"] = msg
	} else if msg := validation.OneOf(u.Role, []string{"super-admin", "admin", "user"}, "Role"); msg != "" {
		e["role"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func (l *UserList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("username", "display_name", "role"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM users"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + userColumns + ` FROM users` + where + ` ORDER BY created_at DESC` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}

	if err := l.ScanRows(rows); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return total, nil
}

func (u *User) Create(passwordHash string) error {
	u.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	u.CreatedAt = now
	u.UpdatedAt = now
	u.PasswordHash = passwordHash

	_, err := DB.Exec(`INSERT INTO users (`+userColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
		u.ID, u.Username, u.PasswordHash, u.DisplayName, u.Role, BoolToInt(u.IsActive), u.CreatedAt, u.UpdatedAt)
	return err
}

func (u *User) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"username": "username", "displayName": "display_name",
		"role": "role", "isActive": "is_active",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			if dbCol == "is_active" {
				if b, ok := val.(bool); ok {
					args = append(args, BoolToInt(b))
				} else {
					args = append(args, val)
				}
			} else {
				args = append(args, val)
			}
		}
	}
	if setClauses == "" {
		return u.GetByID(u.ID)
	}

	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, u.ID)
	_, err := DB.Exec("UPDATE users SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return u.GetByID(u.ID)
}

func (u *User) UpdatePassword(passwordHash string) error {
	_, err := DB.Exec("UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", passwordHash, DateNow(), u.ID)
	return err
}
