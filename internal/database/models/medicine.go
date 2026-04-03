package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

const medicineColumnsNoId = `name, description, created_at`
const medicineColumns = `id, ` + medicineColumnsNoId

var MedicineDeps = map[string]string{
	"patient_medicines": "medicine_id",
}

type Medicine struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   Date   `json:"createdAt"`
}

func (m *Medicine) IsValid() error {
	if msg := validation.Required(m.Name, "Name"); msg != "" {
		return validation.Errors{"name": msg}
	}
	return nil
}

type MedicineList []Medicine

func (m *Medicine) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Medicine row")
	}
	return row.Scan(&m.ID, &m.Name, &m.Description, &m.CreatedAt)
}

func (l *MedicineList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Medicine rows")
	}
	*l = MedicineList{}
	for rows.Next() {
		var item Medicine
		err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (l *MedicineList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name", "description"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM medicines"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + medicineColumns + ` FROM medicines` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	l.ScanRows(rows)

	return total, rows.Err()
}

func GetMedicineDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM medicines` + where + ` ORDER BY name` + params.PaginationClause()
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

func (m *Medicine) GetByID(id string) error {
	return m.ScanRow(RDB.QueryRow(`SELECT `+medicineColumns+` FROM medicines WHERE id = ?`, id))
}

func (m *Medicine) Create() error {
	m.ID = uuid.Must(uuid.NewV7()).String()
	m.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO medicines (`+medicineColumns+`) VALUES (?,?,?,?)`,
		m.ID, m.Name, m.Description, m.CreatedAt)
	return err
}

func (m *Medicine) Update(updates map[string]interface{}) error {
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
		return m.GetByID(m.ID)
	}
	args = append(args, m.ID)
	if _, err := DB.Exec("UPDATE medicines SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}
	return m.GetByID(m.ID)
}

func (m *Medicine) Delete() error {
	res, err := DB.Exec("DELETE FROM medicines WHERE id = ?", m.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
