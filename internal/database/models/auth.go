package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  string `json:"user"`
	Role  string `json:"role"`
}

type Token struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	UserID    string `json:"userId"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
}

func (t *Token) Create() error {
	id := uuid.New().String()
	_, err := DB.Exec(
		`INSERT INTO tokens (id, token, user_id, expires_at, created_at) VALUES (?,?,?,?,?)`,
		id, t.Token, t.UserID, t.ExpiresAt, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (t *Token) GetByValue(tokenStr string) error {
	err := DB.QueryRow(
		`SELECT id, token, user_id, expires_at, created_at FROM tokens WHERE token = ?`, tokenStr,
	).Scan(&t.ID, &t.Token, &t.UserID, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}
	return nil
}

func (t *Token) Delete() error {
	_, err := DB.Exec(`DELETE FROM tokens WHERE token = ?`, t.Token)
	return err
}

func (t *Token) DeleteExpired() error {
	_, err := DB.Exec(`DELETE FROM tokens WHERE expires_at < datetime('now')`)
	return err
}
