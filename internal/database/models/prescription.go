package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type PrescriptionItem struct {
	MedicineID string `json:"medicineId"`
	Dosage     string `json:"dosage"`
	Frequency  string `json:"frequency"`
	Duration   string `json:"duration"`
	Notes      string `json:"notes"`
}

type PrescriptionItems []PrescriptionItem

func (p *PrescriptionItems) Scan(value interface{}) error {
	if value == nil {
		*p = PrescriptionItems{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("unsupported type for PrescriptionItems: %T", value)
	}
	return json.Unmarshal(bytes, p)
}

func (p PrescriptionItems) Value() (driver.Value, error) {
	return json.Marshal(p)
}

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
	if len(p.Items) == 0 {
		e["items"] = "at least one prescription item is required"
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const prescriptionColumnsNoId = `patient_id, patient_procedure_id, visit_id, prescribed_by, prescription_date, items, instructions, status, created_at, updated_at`
const prescriptionColumns = `id, ` + prescriptionColumnsNoId

type Prescription struct {
	ID                 string            `json:"id"`
	PatientID          string            `json:"patientId"`
	PatientProcedureID string            `json:"patientProcedureId"`
	VisitID            string            `json:"visitId"`
	PrescribedBy       string            `json:"prescribedBy"`
	PrescriptionDate   Date              `json:"prescriptionDate"`
	Items              PrescriptionItems `json:"items"`
	Instructions       string            `json:"instructions"`
	Status             string            `json:"status"`
	CreatedAt          Date              `json:"createdAt"`
	UpdatedAt          Date              `json:"updatedAt"`
}

type PrescriptionList []Prescription

func (m *Prescription) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Prescription row")
	}
	err := row.Scan(&m.ID, &m.PatientID, &m.PatientProcedureID, &m.VisitID, &m.PrescribedBy,
		&m.PrescriptionDate, &m.Items, &m.Instructions, &m.Status, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return err
	}
	if m.Items == nil {
		m.Items = PrescriptionItems{}
	}
	return nil
}

func (l *PrescriptionList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Prescription rows")
	}
	*l = PrescriptionList{}
	for rows.Next() {
		var item Prescription
		err := rows.Scan(&item.ID, &item.PatientID, &item.PatientProcedureID, &item.VisitID, &item.PrescribedBy,
			&item.PrescriptionDate, &item.Items, &item.Instructions, &item.Status, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		if item.Items == nil {
			item.Items = PrescriptionItems{}
		}
		*l = append(*l, item)
	}
	return nil
}

func (p *Prescription) GetByPatient(patientID string) ([]Prescription, error) {
	rows, err := DB.Query(`SELECT `+prescriptionColumns+`
		FROM prescriptions WHERE patient_id = ? ORDER BY prescription_date DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list PrescriptionList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, rows.Err()
}

func (p *Prescription) GetByID(id string) error {
	row := DB.QueryRow(`SELECT `+prescriptionColumns+`
		FROM prescriptions WHERE id = ?`, id)
	return p.ScanRow(row)
}

func (p *Prescription) Create() error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.Items == nil {
		p.Items = PrescriptionItems{}
	}
	if p.Status == "" {
		p.Status = "active"
	}

	itemsJSON, err := json.Marshal(p.Items)
	if err != nil {
		return fmt.Errorf("marshal items: %w", err)
	}

	_, err = DB.Exec(`INSERT INTO prescriptions (`+prescriptionColumns+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.PatientID, p.PatientProcedureID, p.VisitID, p.PrescribedBy, p.PrescriptionDate, string(itemsJSON), p.Instructions, p.Status, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (p *Prescription) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"visitId": "visit_id", "prescribedBy": "prescribed_by", "prescriptionDate": "prescription_date",
		"instructions": "instructions", "status": "status",
	}

	setClauses := "updated_at = ?"
	args := []interface{}{DateNow()}

	if items, ok := updates["items"]; ok {
		itemsJSON, err := json.Marshal(items)
		if err != nil {
			return fmt.Errorf("marshal items: %w", err)
		}
		setClauses += ", items = ?"
		args = append(args, string(itemsJSON))
	}

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
	res, err := DB.Exec("DELETE FROM prescriptions WHERE id = ?", p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
