package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type PrescriptionItem struct {
	Name      string `json:"name"`
	Dosage    string `json:"dosage"`
	Frequency string `json:"frequency"`
	Duration  string `json:"duration"`
	Notes     string `json:"notes"`
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

type Prescription struct {
	ID                 string            `json:"id"`
	PatientID          string            `json:"patientId"`
	PatientProcedureID string            `json:"patientProcedureId"`
	VisitID            string            `json:"visitId"`
	PrescribedBy       string            `json:"prescribedBy"`
	PrescriptionDate   string            `json:"prescriptionDate"`
	Items              PrescriptionItems `json:"items"`
	Instructions       string            `json:"instructions"`
	Status             string            `json:"status"`
	CreatedAt          string            `json:"createdAt"`
	UpdatedAt          string            `json:"updatedAt"`
}

func (p *Prescription) GetByPatient(patientID string) ([]Prescription, error) {
	rows, err := DB.Query(`SELECT id, patient_id, patient_procedure_id, visit_id, prescribed_by, prescription_date, items, instructions, status, created_at, updated_at
		FROM prescriptions WHERE patient_id = ? ORDER BY prescription_date DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Prescription
	for rows.Next() {
		var rx Prescription
		if err := rows.Scan(&rx.ID, &rx.PatientID, &rx.PatientProcedureID, &rx.VisitID, &rx.PrescribedBy, &rx.PrescriptionDate, &rx.Items, &rx.Instructions, &rx.Status, &rx.CreatedAt, &rx.UpdatedAt); err != nil {
			return nil, err
		}
		if rx.Items == nil {
			rx.Items = PrescriptionItems{}
		}
		list = append(list, rx)
	}
	return list, rows.Err()
}

func (p *Prescription) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, patient_id, patient_procedure_id, visit_id, prescribed_by, prescription_date, items, instructions, status, created_at, updated_at
		FROM prescriptions WHERE id = ?`, id).Scan(
		&p.ID, &p.PatientID, &p.PatientProcedureID, &p.VisitID, &p.PrescribedBy, &p.PrescriptionDate, &p.Items, &p.Instructions, &p.Status, &p.CreatedAt, &p.UpdatedAt,
	)
	if p.Items == nil {
		p.Items = PrescriptionItems{}
	}
	return err
}

func (p *Prescription) Create() error {
	p.ID = uuid.New().String()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
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

	_, err = DB.Exec(`INSERT INTO prescriptions (id, patient_id, patient_procedure_id, visit_id, prescribed_by, prescription_date, items, instructions, status, created_at, updated_at)
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
	args := []interface{}{time.Now().UTC().Format("2006-01-02 15:04:05")}

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
