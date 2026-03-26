package models

import "fmt"

type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	DisplayName  string `json:"displayName"`
	Role         string `json:"role"`
	IsActive     bool   `json:"isActive"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func (u *User) GetByID(id string) error {
	var isActive int
	err := DB.QueryRow(`SELECT id, username, password_hash, display_name, role, is_active, created_at, updated_at
		FROM users WHERE id = ?`, id).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &isActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("get user by id: %w", err)
	}
	u.IsActive = isActive == 1
	return nil
}

func (u *User) GetByUsername(username string) error {
	var isActive int
	err := DB.QueryRow(`SELECT id, username, password_hash, display_name, role, is_active, created_at, updated_at
		FROM users WHERE username = ?`, username).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &isActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("get user by username: %w", err)
	}
	u.IsActive = isActive == 1
	return nil
}
