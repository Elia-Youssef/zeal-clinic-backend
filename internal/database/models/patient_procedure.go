package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

const patientProcedureColumnsNoId = `patient_id, procedure_id, appointment_id, status, notes, created_at, updated_at`
const patientProcedureColumns = `id, ` + patientProcedureColumnsNoId

type PatientProcedure struct {
	ID            string  `json:"id"`
	PatientID     string  `json:"patientId"`
	ProcedureID   string  `json:"procedureId"`
	AppointmentID *string `json:"appointmentId"`
	Status        string  `json:"status"`
	Notes         string  `json:"notes"`
	CreatedAt     Date    `json:"createdAt"`
	UpdatedAt     Date    `json:"updatedAt"`
	// Nested
	Sessions PatientProcedureSessionList `json:"sessions,omitempty"`
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
	err := row.Scan(&m.ID, &m.PatientID, &m.ProcedureID, &m.AppointmentID, &m.Status, &m.Notes,
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
		err := rows.Scan(&item.ID, &item.PatientID, &item.ProcedureID, &item.AppointmentID, &item.Status, &item.Notes,
			&item.CreatedAt, &item.UpdatedAt, &item.ProcedureName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (pp *PatientProcedureList) GetByProcedure(procedureID string) error {
	rows, err := RDB.Query(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.appointment_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.procedure_id = ? ORDER BY pp.created_at DESC`, procedureID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := pp.ScanRows(rows); err != nil {
		return err
	}
	for i := range *pp {
		(*pp)[i].Sessions.GetByPatientProcedure((*pp)[i].ID)
	}
	return nil
}

func (pp *PatientProcedureList) GetByPatient(patientID string) error {
	rows, err := RDB.Query(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.appointment_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.patient_id = ? ORDER BY pp.created_at DESC`, patientID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := pp.ScanRows(rows); err != nil {
		return err
	}

	for i := range *pp {
		(*pp)[i].Sessions.GetByPatientProcedure((*pp)[i].ID)
	}

	return nil
}

func (pp *PatientProcedure) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.appointment_id, pp.status, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.id = ?`, id)
	if err := pp.ScanRow(row); err != nil {
		return err
	}
	pp.Sessions.GetByPatientProcedure(pp.ID)
	return nil
}

func (pp *PatientProcedure) Create() error {
	if err := pp.create(DB); err != nil {
		return err
	}
	return pp.GetByID(pp.ID)
}

// create inserts the patient_procedure and auto-creates sessions. It accepts
// a DBTX so it can run inside an existing transaction (e.g. from
// Appointment.Create) without deadlocking on the single SQLite connection.
func (pp *PatientProcedure) create(db DBTX) error {
	pp.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	pp.CreatedAt = now
	pp.UpdatedAt = now
	if pp.Status == "" {
		pp.Status = "planned"
	}
	_, err := db.Exec(`INSERT INTO patient_procedures (`+patientProcedureColumns+`)
		VALUES (?,?,?,?,?,?,?,?)`,
		pp.ID, pp.PatientID, pp.ProcedureID, pp.AppointmentID, pp.Status, pp.Notes, pp.CreatedAt, pp.UpdatedAt)
	if err != nil {
		return err
	}

	// Collect session IDs first so the rows are closed before we insert.
	// Holding rows open while calling Exec deadlocks with MaxOpenConns(1).
	rows, err := db.Query(`SELECT id FROM procedure_sessions WHERE procedure_id = ? ORDER BY session_number`, pp.ProcedureID)
	if err != nil {
		return nil
	}
	var sessionIDs []string
	for rows.Next() {
		var psID string
		if err := rows.Scan(&psID); err != nil {
			continue
		}
		sessionIDs = append(sessionIDs, psID)
	}
	rows.Close()

	for _, psID := range sessionIDs {
		sessID := uuid.Must(uuid.NewV7()).String()
		if _, err := db.Exec(`INSERT INTO patient_procedure_sessions (id, patient_procedure_id, procedure_session_id, appointment_id, status, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?)`, sessID, pp.ID, psID, nil, "pending", now, now); err != nil {
			return err
		}
	}
	return nil
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
