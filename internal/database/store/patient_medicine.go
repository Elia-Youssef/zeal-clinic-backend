package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

const patientMedicineColumnsNoId = `patient_id, medicine_id, is_active, notes, created_at`
const patientMedicineColumns = `id, ` + patientMedicineColumnsNoId

type PatientMedicine struct {
	ID         string `json:"id"`
	PatientID  string `json:"patientId"`
	MedicineID string `json:"medicineId"`
	IsActive   bool   `json:"isActive"`
	Notes      string `json:"notes"`
	CreatedAt  Date   `json:"createdAt"`
	// Joined fields
	MedicineName string `json:"medicineName,omitempty"`
}

func (pm *PatientMedicine) IsValid() error {
	if msg := validation.Required(pm.MedicineID, "Medicine ID"); msg != "" {
		return validation.Errors{"medicineId": msg}
	}
	return nil
}

type PatientMedicineList []PatientMedicine

func (m *PatientMedicine) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil PatientMedicine row")
	}
	var isActive int
	err := row.Scan(&m.ID, &m.PatientID, &m.MedicineID, &isActive, &m.Notes, &m.CreatedAt, &m.MedicineName)
	if err != nil {
		return err
	}
	m.IsActive = isActive == 1
	return nil
}

func (l *PatientMedicineList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil PatientMedicine rows")
	}
	*l = PatientMedicineList{}
	for rows.Next() {
		var item PatientMedicine
		var isActive int
		err := rows.Scan(&item.ID, &item.PatientID, &item.MedicineID, &isActive, &item.Notes, &item.CreatedAt, &item.MedicineName)
		if err != nil {
			continue
		}
		item.IsActive = isActive == 1
		*l = append(*l, item)
	}
	return nil
}

func (pm *PatientMedicineList) GetByPatient(patientID string) error {
	rows, err := RDB.Query(`SELECT pm.id, pm.patient_id, pm.medicine_id, pm.is_active, pm.notes, pm.created_at, m.name
		FROM patient_medicines pm JOIN medicines m ON m.id = pm.medicine_id
		WHERE pm.patient_id = ? ORDER BY m.name`, patientID)
	if err != nil {
		return err
	}
	defer rows.Close()

	return pm.ScanRows(rows)
}

func (pm *PatientMedicine) Create() error {
	pm.ID = uuid.Must(uuid.NewV7()).String()
	pm.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO patient_medicines (id, patient_id, medicine_id, is_active, notes, created_at) VALUES (?,?,?,?,?,?)`,
		pm.ID, pm.PatientID, pm.MedicineID, BoolToInt(pm.IsActive), pm.Notes, pm.CreatedAt)
	return err
}

func (pm *PatientMedicine) UpdateNotes() error {
	res, err := DB.Exec("UPDATE patient_medicines SET notes = ? WHERE id = ?", pm.Notes, pm.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (pm *PatientMedicine) Delete() error {
	res, err := DB.Exec("DELETE FROM patient_medicines WHERE id = ?", pm.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
