package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

func (p *Prescription) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(p.PatientID, "Patient ID"); msg != "" {
		e["patientId"] = msg
	}
	if msg := validation.Required(string(p.PrescriptionDate), "Prescription date"); msg != "" {
		e["prescriptionDate"] = msg
	} else if msg := validation.Date(string(p.PrescriptionDate)); msg != "" {
		e["prescriptionDate"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const prescriptionColumnsNoId = `patient_id, patient_procedure_id, prescribed_by, prescription_date, created_at, updated_at`
const prescriptionColumns = `id, ` + prescriptionColumnsNoId

type Prescription struct {
	ID                 string                   `json:"id"`
	PatientID          string                   `json:"patientId"`
	PatientProcedureID string                   `json:"patientProcedureId"`
	PrescribedBy       string                   `json:"prescribedBy"`
	PrescriptionDate   Date                     `json:"prescriptionDate"`
	CreatedAt          Date                     `json:"createdAt"`
	UpdatedAt          Date                     `json:"updatedAt"`
	Medicines          PrescriptionMedicineList `json:"medicines,omitempty"`
}

type PrescriptionList []Prescription

func (m *Prescription) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Prescription row")
	}
	return row.Scan(&m.ID, &m.PatientID, &m.PatientProcedureID, &m.PrescribedBy,
		&m.PrescriptionDate, &m.CreatedAt, &m.UpdatedAt)
}

func (l *PrescriptionList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Prescription rows")
	}
	*l = PrescriptionList{}
	for rows.Next() {
		var item Prescription
		err := rows.Scan(&item.ID, &item.PatientID, &item.PatientProcedureID, &item.PrescribedBy,
			&item.PrescriptionDate, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (p *Prescription) GetByPatient(patientID string) ([]Prescription, error) {
	rows, err := RDB.Query(`SELECT `+prescriptionColumns+`
		FROM prescriptions WHERE patient_id = ? ORDER BY prescription_date DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list PrescriptionList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}

	for i := range list {
		meds, err := (&PrescriptionMedicine{}).GetByPrescription(list[i].ID)
		if err != nil {
			return nil, err
		}
		list[i].Medicines = meds
	}

	return list, rows.Err()
}

func (p *Prescription) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+prescriptionColumns+`
		FROM prescriptions WHERE id = ?`, id)
	if err := p.ScanRow(row); err != nil {
		return err
	}

	meds, err := (&PrescriptionMedicine{}).GetByPrescription(p.ID)
	if err != nil {
		return err
	}
	p.Medicines = meds
	return nil
}

func (p *Prescription) Create() error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := DB.Exec(`INSERT INTO prescriptions (`+prescriptionColumns+`)
		VALUES (?,?,?,?,?,?,?)`,
		p.ID, p.PatientID, p.PatientProcedureID, p.PrescribedBy, p.PrescriptionDate, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (p *Prescription) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"prescribedBy": "prescribed_by", "prescriptionDate": "prescription_date",
		"patientProcedureId": "patient_procedure_id",
	}

	setClauses := "updated_at = ?"
	args := []interface{}{DateNow()}

	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			setClauses += ", " + dbCol + " = ?"
			args = append(args, val)
		}
	}

	args = append(args, p.ID)
	_, err := DB.Exec("UPDATE prescriptions SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return p.GetByID(p.ID)
}

func (p *Prescription) Delete() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM prescription_medicines WHERE prescription_id = ?", p.ID); err != nil {
		return err
	}
	res, err := tx.Exec("DELETE FROM prescriptions WHERE id = ?", p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}
