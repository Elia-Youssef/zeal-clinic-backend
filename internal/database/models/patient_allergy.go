package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// Patient Allergy

const patientAllergyColumnsNoId = `patient_id, allergy_id, notes, created_at`
const patientAllergyColumns = `id, ` + patientAllergyColumnsNoId

type PatientAllergy struct {
	ID        string `json:"id"`
	PatientID string `json:"patientId"`
	AllergyID string `json:"allergyId"`
	Notes     string `json:"notes"`
	CreatedAt Date   `json:"createdAt"`
	// Joined fields
	AllergyName string `json:"allergyName,omitempty"`
}

func (pa *PatientAllergy) IsValid() error {
	if msg := validation.Required(pa.AllergyID, "Allergy ID"); msg != "" {
		return validation.Errors{"allergyId": msg}
	}
	return nil
}

type PatientAllergyList []PatientAllergy

func (m *PatientAllergy) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil PatientAllergy row")
	}
	return row.Scan(&m.ID, &m.PatientID, &m.AllergyID, &m.Notes, &m.CreatedAt, &m.AllergyName)
}

func (l *PatientAllergyList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil PatientAllergy rows")
	}
	*l = PatientAllergyList{}
	for rows.Next() {
		var item PatientAllergy
		err := rows.Scan(&item.ID, &item.PatientID, &item.AllergyID, &item.Notes, &item.CreatedAt, &item.AllergyName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (pa *PatientAllergy) GetByPatient(patientID string) ([]PatientAllergy, error) {
	rows, err := DB.Query(`SELECT pa.id, pa.patient_id, pa.allergy_id, pa.notes, pa.created_at, a.name
		FROM patient_allergies pa JOIN allergies a ON a.id = pa.allergy_id
		WHERE pa.patient_id = ? ORDER BY a.name`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list PatientAllergyList
	list.ScanRows(rows)
	return list, rows.Err()
}

func (pa *PatientAllergy) Create() error {
	pa.ID = uuid.Must(uuid.NewV7()).String()
	pa.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO patient_allergies (id, patient_id, allergy_id, notes, created_at) VALUES (?,?,?,?,?)`,
		pa.ID, pa.PatientID, pa.AllergyID, pa.Notes, pa.CreatedAt)
	return err
}

func (pa *PatientAllergy) Delete() error {
	res, err := DB.Exec("DELETE FROM patient_allergies WHERE id = ?", pa.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
