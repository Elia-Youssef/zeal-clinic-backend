package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// Allergy

const allergyColumnsNoId = `name, description, created_at`
const allergyColumns = `id, ` + allergyColumnsNoId

type Allergy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   Date   `json:"createdAt"`
}

func (a *Allergy) IsValid() error {
	if msg := validation.Required(a.Name, "Name"); msg != "" {
		return validation.Errors{"name": msg}
	}
	return nil
}

type AllergyList []Allergy

func (m *Allergy) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Allergy row")
	}
	return row.Scan(&m.ID, &m.Name, &m.Description, &m.CreatedAt)
}

func (l *AllergyList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Allergy rows")
	}
	*l = AllergyList{}
	for rows.Next() {
		var item Allergy
		err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (a *Allergy) GetAll() ([]Allergy, error) {
	rows, err := DB.Query(`SELECT id, name, description, created_at FROM allergies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list AllergyList
	list.ScanRows(rows)
	return list, rows.Err()
}

func (a *Allergy) GetByID(id string) error {
	return a.ScanRow(DB.QueryRow(`SELECT id, name, description, created_at FROM allergies WHERE id = ?`, id))
}

func (a *Allergy) Create() error {
	a.ID = uuid.Must(uuid.NewV7()).String()
	a.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO allergies (id, name, description, created_at) VALUES (?,?,?,?)`,
		a.ID, a.Name, a.Description, a.CreatedAt)
	return err
}

func (a *Allergy) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "description": "description",
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
		return a.GetByID(a.ID)
	}
	args = append(args, a.ID)
	if _, err := DB.Exec("UPDATE allergies SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}
	return a.GetByID(a.ID)
}

func (a *Allergy) Delete() error {
	res, err := DB.Exec("DELETE FROM allergies WHERE id = ?", a.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
