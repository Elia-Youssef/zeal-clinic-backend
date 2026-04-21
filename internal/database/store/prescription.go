package store

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
	if msg := validation.Required(string(p.StartDate), "Start date"); msg != "" {
		e["startDate"] = msg
	} else if msg := validation.Date(string(p.StartDate)); msg != "" {
		e["startDate"] = msg
	}
	if string(p.EndDate) != "" {
		if msg := validation.Date(string(p.EndDate)); msg != "" {
			e["endDate"] = msg
		}
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const prescriptionColumnsNoId = `patient_id, prescribed_by_id, start_date, end_date, created_at, updated_at`
const prescriptionColumns = `id, ` + prescriptionColumnsNoId

type Prescription struct {
	ID             string                   `json:"id"`
	PatientID      string                   `json:"patientId"`
	PrescribedByID string                   `json:"prescribedById"`
	StartDate      Date                     `json:"startDate"`
	EndDate        Date                     `json:"endDate"`
	CreatedAt      Date                     `json:"createdAt"`
	UpdatedAt      Date                     `json:"updatedAt"`
	Medicines      PrescriptionMedicineList `json:"medicines,omitempty"`
	// Joined fields
	PrescribedByName string `json:"prescribedByName,omitempty"`
}

type PrescriptionList []Prescription

func (m *Prescription) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Prescription row")
	}
	return row.Scan(&m.ID, &m.PatientID, &m.PrescribedByID,
		&m.StartDate, &m.EndDate, &m.CreatedAt, &m.UpdatedAt, &m.PrescribedByName)
}

func (l *PrescriptionList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Prescription rows")
	}
	*l = PrescriptionList{}
	for rows.Next() {
		var item Prescription
		err := rows.Scan(&item.ID, &item.PatientID, &item.PrescribedByID,
			&item.StartDate, &item.EndDate, &item.CreatedAt, &item.UpdatedAt, &item.PrescribedByName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

const prescriptionSelectQuery = `SELECT p.id, p.patient_id, p.prescribed_by_id, p.start_date, p.end_date, p.created_at, p.updated_at,
	COALESCE(e.first_name || ' ' || e.last_name, '') AS prescribed_by_name
	FROM prescriptions p
	LEFT JOIN employees e ON e.id = p.prescribed_by_id`

func (p *PrescriptionList) GetByPatient(patientID string) error {
	rows, err := RDB.Query(prescriptionSelectQuery+`
		WHERE p.patient_id = ? ORDER BY p.start_date DESC`, patientID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := p.ScanRows(rows); err != nil {
		return err
	}

	for i := range *p {
		if err := (*p)[i].Medicines.GetByPrescription((*p)[i].ID); err != nil {
			return err
		}
	}

	return rows.Err()
}

func (p *Prescription) GetByID(id string) error {
	row := RDB.QueryRow(prescriptionSelectQuery+`
		WHERE p.id = ?`, id)
	if err := p.ScanRow(row); err != nil {
		return err
	}

	if err := p.Medicines.GetByPrescription(p.ID); err != nil {
		return err
	}
	return nil
}

func (p *Prescription) Create() error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	p.CreatedAt = now
	p.UpdatedAt = now

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO prescriptions (`+prescriptionColumns+`)
		VALUES (?,?,?,?,?,?,?)`,
		p.ID, p.PatientID, p.PrescribedByID, p.StartDate, p.EndDate, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return err
	}

	for i := range p.Medicines {
		m := &p.Medicines[i]
		m.ID = uuid.Must(uuid.NewV7()).String()
		m.PrescriptionID = p.ID
		m.CreatedAt = now
		if m.Status == "" {
			m.Status = "active"
		}
		_, err = tx.Exec(`INSERT INTO prescription_medicines (`+prescriptionMedicineColumns+`) VALUES (?,?,?,?,?,?)`,
			m.ID, m.MedicineID, m.PrescriptionID, m.Instructions, m.Status, m.CreatedAt)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return p.GetByID(p.ID)
}

func (p *Prescription) Update(updates map[string]any) error {
	cols := map[string]string{
		"prescribedById": "prescribed_by_id", "startDate": "start_date", "endDate": "end_date",
	}

	setClauses := "updated_at = ?"
	args := []any{DateNow()}

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
		return ErrNotFound
	}
	return tx.Commit()
}
