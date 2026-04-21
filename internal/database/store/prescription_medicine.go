package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

const prescriptionMedicineColumnsNoId = `medicine_id, prescription_id, instructions, status, created_at`
const prescriptionMedicineColumns = `id, ` + prescriptionMedicineColumnsNoId

type PrescriptionMedicine struct {
	ID             string `json:"id"`
	MedicineID     string `json:"medicineId"`
	PrescriptionID string `json:"prescriptionId"`
	Instructions   string `json:"instructions"`
	Status         string `json:"status"`
	CreatedAt      Date   `json:"createdAt"`
	// Joined fields
	MedicineName string `json:"medicineName,omitempty"`
}

type PrescriptionMedicineList []PrescriptionMedicine

func (pm *PrescriptionMedicine) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(pm.MedicineID, "Medicine ID"); msg != "" {
		e["medicineId"] = msg
	}
	if msg := validation.Required(pm.PrescriptionID, "Prescription ID"); msg != "" {
		e["prescriptionId"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func (m *PrescriptionMedicine) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil PrescriptionMedicine row")
	}
	return row.Scan(&m.ID, &m.MedicineID, &m.PrescriptionID, &m.Instructions, &m.Status, &m.CreatedAt)
}

func (l *PrescriptionMedicineList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil PrescriptionMedicine rows")
	}
	*l = PrescriptionMedicineList{}
	for rows.Next() {
		var item PrescriptionMedicine
		err := rows.Scan(&item.ID, &item.MedicineID, &item.PrescriptionID, &item.Instructions, &item.Status, &item.CreatedAt, &item.MedicineName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (pm *PrescriptionMedicineList) GetByPrescription(prescriptionID string) error {
	rows, err := RDB.Query(`SELECT pm.id, pm.medicine_id, pm.prescription_id, pm.instructions, pm.status, pm.created_at,
		m.name
		FROM prescription_medicines pm
		JOIN medicines m ON m.id = pm.medicine_id
		WHERE pm.prescription_id = ? ORDER BY pm.created_at`, prescriptionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := pm.ScanRows(rows); err != nil {
		return err
	}
	return rows.Err()
}

func (pm *PrescriptionMedicine) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+prescriptionMedicineColumns+` FROM prescription_medicines WHERE id = ?`, id)
	return pm.ScanRow(row)
}

func (pm *PrescriptionMedicine) Create() error {
	pm.ID = uuid.Must(uuid.NewV7()).String()
	pm.CreatedAt = DateNow()
	if pm.Status == "" {
		pm.Status = "active"
	}
	_, err := DB.Exec(`INSERT INTO prescription_medicines (`+prescriptionMedicineColumns+`) VALUES (?,?,?,?,?,?)`,
		pm.ID, pm.MedicineID, pm.PrescriptionID, pm.Instructions, pm.Status, pm.CreatedAt)
	return err
}

func (pm *PrescriptionMedicine) Update(updates map[string]any) error {
	cols := map[string]string{
		"instructions": "instructions", "status": "status",
	}
	setClauses := ""
	var args []any
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
		return ErrNotFound
	}
	return nil
}
