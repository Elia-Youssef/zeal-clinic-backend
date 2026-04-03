package models

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Token struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	UserID    string `json:"userId"`
	ExpiresAt Date `json:"expiresAt"`
	CreatedAt Date `json:"createdAt"`
}

const tokenColumnsNoId = `token, user_id, expires_at, created_at`
const tokenColumns = `id, ` + tokenColumnsNoId

type TokenList []Token

func (m *Token) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Token row")
	}
	return row.Scan(&m.ID, &m.Token, &m.UserID, &m.ExpiresAt, &m.CreatedAt)
}

func (l *TokenList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Token rows")
	}
	*l = TokenList{}
	for rows.Next() {
		var item Token
		err := rows.Scan(&item.ID, &item.Token, &item.UserID, &item.ExpiresAt, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (t *Token) Create() error {
	id := uuid.Must(uuid.NewV7()).String()
	_, err := DB.Exec(
		`INSERT INTO tokens (`+tokenColumns+`) VALUES (?,?,?,?,?)`,
		id, t.Token, t.UserID, t.ExpiresAt, DateNow(),
	)
	return err
}

func (t *Token) GetByValue(tokenStr string) error {
	err := t.ScanRow(RDB.QueryRow(
		`SELECT `+tokenColumns+` FROM tokens WHERE token = ?`, tokenStr,
	))
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
