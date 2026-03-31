package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

const patientProcedureColumnsNoId = `patient_id, procedure_id, status, notes, created_at, updated_at`
const patientProcedureColumns = `id, ` + patientProcedureColumnsNoId

type PatientProcedure struct {
	ID          string `json:"id"`
	PatientID   string `json:"patientId"`
	ProcedureID string `json:"procedureId"`
	Status      string `json:"status"`
	Notes       string `json:"notes"`
	CreatedAt   Date   `json:"createdAt"`
	UpdatedAt   Date   `json:"updatedAt"`
	// Nested
	Sessions []PatientProcedureSession `json:"sessions,omitempty"`
	// Joined fields
	ProcedureName string `json:"procedureName,omitempty"`
	PatientName   string `json:"patientName,omitempty"`
}

func (pp *PatientProcedure) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(pp.PatientID, "Patient ID"); msg != "" {
		e["patientId"] = msg
	}
	if msg := validation.Required(pp.ProcedureID, "Procedure ID"); msg != "" {
		e["procedureId"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

type PatientProcedureList []PatientProcedure

func (m *PatientProcedure) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil PatientProcedure row")
	}
	err := row.Scan(&m.ID, &m.PatientID, &m.ProcedureID, &m.Status, &m.Notes,
		&m.CreatedAt, &m.UpdatedAt, &m.ProcedureName)
	if err != nil {
		return err
	}
	return nil
}

func (l *PatientProcedureList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil PatientProcedure rows")
	}
	*l = PatientProcedureList{}
	for rows.Next() {
		var item PatientProcedure
		err := rows.Scan(&item.ID, &item.PatientID, &item.ProcedureID, &item.Status, &item.Notes,
			&item.CreatedAt, &item.UpdatedAt, &item.ProcedureName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (pp *PatientProcedure) GetByProcedure(procedureID string) ([]PatientProcedure, error) {
	rows, err := DB.Query(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.procedure_id = ? ORDER BY pp.created_at DESC`, procedureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items PatientProcedureList
	if err := items.ScanRows(rows); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Sessions, _ = (&PatientProcedureSession{}).GetByPatientProcedure(items[i].ID)
	}
	return items, rows.Err()
}

func (pp *PatientProcedure) GetByPatient(patientID string) ([]PatientProcedure, error) {
	rows, err := DB.Query(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.patient_id = ? ORDER BY pp.created_at DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items PatientProcedureList
	if err := items.ScanRows(rows); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Sessions, _ = (&PatientProcedureSession{}).GetByPatientProcedure(items[i].ID)
	}
	return items, rows.Err()
}

func (pp *PatientProcedure) GetByID(id string) error {
	row := DB.QueryRow(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.id = ?`, id)
	if err := pp.ScanRow(row); err != nil {
		return err
	}
	pp.Sessions, _ = (&PatientProcedureSession{}).GetByPatientProcedure(pp.ID)
	return nil
}

func (pp *PatientProcedure) Create() error {
	pp.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	pp.CreatedAt = now
	pp.UpdatedAt = now
	if pp.Status == "" {
		pp.Status = "planned"
	}
	_, err := DB.Exec(`INSERT INTO patient_procedures (`+patientProcedureColumns+`)
		VALUES (?,?,?,?,?,?,?)`,
		pp.ID, pp.PatientID, pp.ProcedureID, pp.Status, pp.Notes, pp.CreatedAt, pp.UpdatedAt)
	if err != nil {
		return err
	}
	// Auto-create sessions based on procedure_sessions template
	procSessions, _ := DB.Query(`SELECT id FROM procedure_sessions WHERE procedure_id = ? ORDER BY session_number`, pp.ProcedureID)
	if procSessions != nil {
		defer procSessions.Close()
		for procSessions.Next() {
			var psID string
			if err := procSessions.Scan(&psID); err != nil {
				continue
			}
			sessID := uuid.Must(uuid.NewV7()).String()
			DB.Exec(`INSERT INTO patient_procedure_sessions (id, patient_procedure_id, procedure_session_id, status, created_at, updated_at)
				VALUES (?,?,?,?,?,?)`, sessID, pp.ID, psID, "pending", now, now)
		}
	}
	return pp.GetByID(pp.ID)
}

func (pp *PatientProcedure) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"status": "status", "notes": "notes",
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
		return pp.GetByID(pp.ID)
	}
	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, pp.ID)
	_, err := DB.Exec("UPDATE patient_procedures SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return pp.GetByID(pp.ID)
}

func (pp *PatientProcedure) Delete() error {
	res, err := DB.Exec("DELETE FROM patient_procedures WHERE id = ?", pp.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
