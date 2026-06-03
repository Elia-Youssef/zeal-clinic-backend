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
	if msg := validation.Required(p.PrescribedByID, "Prescribed by"); msg != "" {
		e["prescribedById"] = msg
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
	for i := range p.Medicines {
		if msg := validation.Required(p.Medicines[i].MedicineID, "Medicine ID"); msg != "" {
			e["medicines"] = msg
			break
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
			return err
		}
		*l = append(*l, item)
	}
	return nil
}

const prescriptionSelectQuery = `SELECT p.id, p.patient_id, p.prescribed_by_id, p.start_date, p.end_date, p.created_at, p.updated_at,
	COALESCE(e.first_name || ' ' || e.last_name, '') AS prescribed_by_name
	FROM prescriptions p
	LEFT JOIN employees e ON e.id = p.prescribed_by_id`

func (p *PrescriptionList) GetByPatient(patientID string, params ListParams) (int, error) {
	var total int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM prescriptions WHERE patient_id = ?`, patientID).Scan(&total); err != nil {
		return 0, err
	}

	rows, err := RDB.Query(prescriptionSelectQuery+`
		WHERE p.patient_id = ? ORDER BY p.start_date DESC`+params.PaginationClause(), patientID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := p.ScanRows(rows); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	byPrescription, err := MedicinesByPatient(patientID)
	if err != nil {
		return 0, err
	}
	for i := range *p {
		(*p)[i].Medicines = byPrescription[(*p)[i].ID]
	}

	return total, nil
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

	if err := p.insertMedicines(tx, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return p.GetByID(p.ID)
}

// insertMedicines assigns IDs and inserts each medicine in p.Medicines within tx.
func (p *Prescription) insertMedicines(tx *sql.Tx, now Date) error {
	for i := range p.Medicines {
		m := &p.Medicines[i]
		m.ID = uuid.Must(uuid.NewV7()).String()
		m.PrescriptionID = p.ID
		m.CreatedAt = now
		if _, err := tx.Exec(`INSERT INTO prescription_medicines (`+prescriptionMedicineColumns+`) VALUES (?,?,?,?,?)`,
			m.ID, m.MedicineID, m.PrescriptionID, m.Instructions, m.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

// Update rewrites the prescription header and replaces its medicine list in a
// single transaction: the edit form submits the whole prescription at once.
// patient_id and created_at are immutable here.
func (p *Prescription) Update() error {
	now := DateNow()

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE prescriptions
		SET prescribed_by_id = ?, start_date = ?, end_date = ?, updated_at = ?
		WHERE id = ?`,
		p.PrescribedByID, p.StartDate, p.EndDate, now, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	if _, err := tx.Exec("DELETE FROM prescription_medicines WHERE prescription_id = ?", p.ID); err != nil {
		return err
	}
	if err := p.insertMedicines(tx, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
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
