package models

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

const prescriptionMedicineColumnsNoId = `name, generic_name, form, created_at`
const prescriptionMedicineColumns = `id, ` + prescriptionMedicineColumnsNoId

type PrescriptionMedicine struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	GenericName string `json:"genericName"`
	Form        string `json:"form"`
	CreatedAt   Date   `json:"createdAt"`
}

type PrescriptionMedicineList []PrescriptionMedicine

func (m *PrescriptionMedicine) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil PrescriptionMedicine row")
	}
	err := row.Scan(&m.ID, &m.Name, &m.GenericName, &m.Form, &m.CreatedAt)
	if err != nil {
		return err
	}
	return nil
}

func (l *PrescriptionMedicineList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil PrescriptionMedicine rows")
	}
	*l = PrescriptionMedicineList{}
	for rows.Next() {
		var item PrescriptionMedicine
		err := rows.Scan(&item.ID, &item.Name, &item.GenericName, &item.Form, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (pm *PrescriptionMedicine) GetAll() ([]PrescriptionMedicine, error) {
	rows, err := DB.Query(`SELECT ` + prescriptionMedicineColumns + ` FROM prescription_medicines ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items PrescriptionMedicineList
	if err := items.ScanRows(rows); err != nil {
		return nil, err
	}
	return items, rows.Err()
}

func (pm *PrescriptionMedicine) GetByID(id string) error {
	row := DB.QueryRow(`SELECT `+prescriptionMedicineColumns+` FROM prescription_medicines WHERE id = ?`, id)
	return pm.ScanRow(row)
}

func (pm *PrescriptionMedicine) Create() error {
	pm.ID = uuid.Must(uuid.NewV7()).String()
	pm.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO prescription_medicines (`+prescriptionMedicineColumns+`) VALUES (?,?,?,?,?)`,
		pm.ID, pm.Name, pm.GenericName, pm.Form, pm.CreatedAt)
	return err
}

func (pm *PrescriptionMedicine) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "genericName": "generic_name", "form": "form",
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
		return pm.GetByID(pm.ID)
	}
	args = append(args, pm.ID)
	_, err := DB.Exec("UPDATE prescription_medicines SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return pm.GetByID(pm.ID)
}

func (pm *PrescriptionMedicine) Delete() error {
	res, err := DB.Exec("DELETE FROM prescription_medicines WHERE id = ?", pm.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
