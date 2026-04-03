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

var AllergyDeps = map[string]string{
	"patient_allergies":           "allergy_id",
	"procedure_allergy_conflicts": "allergy_id",
	"product_allergy_conflicts":   "allergy_id",
}

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

func (a *AllergyList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name", "description"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM allergies"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT id, name, description, created_at FROM allergies` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	a.ScanRows(rows)

	return total, rows.Err()
}

func GetAllergyDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM allergies` + where + ` ORDER BY name` + params.PaginationClause()
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

func (a *Allergy) GetByID(id string) error {
	return a.ScanRow(RDB.QueryRow(`SELECT id, name, description, created_at FROM allergies WHERE id = ?`, id))
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
