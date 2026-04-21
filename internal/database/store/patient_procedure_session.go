package store

import (
	"database/sql"
	"errors"
)

const patientProcedureSessionColumnsNoId = `patient_procedure_id, procedure_session_id, appointment_id, status, notes, created_at, updated_at`
const patientProcedureSessionColumns = `id, ` + patientProcedureSessionColumnsNoId

type PatientProcedureSession struct {
	ID                 string  `json:"id"`
	PatientProcedureID string  `json:"patientProcedureId"`
	ProcedureSessionID string  `json:"procedureSessionId"`
	AppointmentID      *string `json:"appointmentId"`
	Status             string  `json:"status"`
	Notes              string  `json:"notes"`
	CreatedAt          Date    `json:"createdAt"`
	UpdatedAt          Date    `json:"updatedAt"`
}

type PatientProcedureSessionList []PatientProcedureSession

func (m *PatientProcedureSession) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil PatientProcedureSession row")
	}
	err := row.Scan(&m.ID, &m.PatientProcedureID, &m.ProcedureSessionID,
		&m.AppointmentID, &m.Status, &m.Notes, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return err
	}
	return nil
}

func (l *PatientProcedureSessionList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil PatientProcedureSession rows")
	}
	*l = PatientProcedureSessionList{}
	for rows.Next() {
		var item PatientProcedureSession
		err := rows.Scan(&item.ID, &item.PatientProcedureID, &item.ProcedureSessionID,
			&item.AppointmentID, &item.Status, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (s *PatientProcedureSessionList) GetByPatientProcedure(patientProcedureID string) error {
	rows, err := RDB.Query(`SELECT `+patientProcedureSessionColumns+`
		FROM patient_procedure_sessions WHERE patient_procedure_id = ? ORDER BY created_at`, patientProcedureID)
	if err != nil {
		return err
	}
	defer rows.Close()

	return s.ScanRows(rows)
}

func (s *PatientProcedureSession) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+patientProcedureSessionColumns+`
		FROM patient_procedure_sessions WHERE id = ?`, id)
	return s.ScanRow(row)
}

func (s *PatientProcedureSession) Update(updates map[string]any) error {
	cols := map[string]string{
		"status": "status", "notes": "notes",
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
		return s.GetByID(s.ID)
	}
	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, s.ID)
	_, err := DB.Exec("UPDATE patient_procedure_sessions SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return s.GetByID(s.ID)
}
