package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type PatientProcedure struct {
	ID                     string  `json:"id"`
	PatientID              string  `json:"patientId"`
	ProcedureID            string  `json:"procedureId"`
	EmployeeID             string  `json:"employeeId"`
	Status                 string  `json:"status"`
	ReferredByPatientID    string  `json:"referredByPatientId"`
	ReferredByExternal     string  `json:"referredByExternal"`
	ReferralCommissionRate float64 `json:"referralCommissionRate"`
	Notes                  string  `json:"notes"`
	CreatedAt              string  `json:"createdAt"`
	UpdatedAt              string  `json:"updatedAt"`
	// Nested
	Sessions []PatientProcedureSession `json:"sessions,omitempty"`
	// Joined fields
	ProcedureName string `json:"procedureName,omitempty"`
	PatientName   string `json:"patientName,omitempty"`
	EmployeeName  string `json:"employeeName,omitempty"`
}

type PatientProcedureSession struct {
	ID                 string `json:"id"`
	PatientProcedureID string `json:"patientProcedureId"`
	ProcedureSessionID string `json:"procedureSessionId"`
	AppointmentID      string `json:"appointmentId"`
	Status             string `json:"status"`
	SessionNumber      int    `json:"sessionNumber"`
	ScheduledDate      string `json:"scheduledDate"`
	CompletedDate      string `json:"completedDate"`
	Notes              string `json:"notes"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
}

func (pp *PatientProcedure) GetByPatient(patientID string) ([]PatientProcedure, error) {
	rows, err := DB.Query(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.employee_id, pp.status,
		pp.referred_by_patient_id, pp.referred_by_external, pp.referral_commission_rate, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.patient_id = ? ORDER BY pp.created_at DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PatientProcedure
	for rows.Next() {
		var item PatientProcedure
		if err := rows.Scan(&item.ID, &item.PatientID, &item.ProcedureID, &item.EmployeeID, &item.Status,
			&item.ReferredByPatientID, &item.ReferredByExternal, &item.ReferralCommissionRate, &item.Notes,
			&item.CreatedAt, &item.UpdatedAt, &item.ProcedureName); err != nil {
			return nil, err
		}
		item.Sessions, _ = (&PatientProcedureSession{}).GetByPatientProcedure(item.ID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (pp *PatientProcedure) GetByID(id string) error {
	err := DB.QueryRow(`SELECT pp.id, pp.patient_id, pp.procedure_id, pp.employee_id, pp.status,
		pp.referred_by_patient_id, pp.referred_by_external, pp.referral_commission_rate, pp.notes,
		pp.created_at, pp.updated_at, pr.name
		FROM patient_procedures pp
		JOIN procedures pr ON pr.id = pp.procedure_id
		WHERE pp.id = ?`, id).
		Scan(&pp.ID, &pp.PatientID, &pp.ProcedureID, &pp.EmployeeID, &pp.Status,
			&pp.ReferredByPatientID, &pp.ReferredByExternal, &pp.ReferralCommissionRate, &pp.Notes,
			&pp.CreatedAt, &pp.UpdatedAt, &pp.ProcedureName)
	if err != nil {
		return err
	}
	pp.Sessions, _ = (&PatientProcedureSession{}).GetByPatientProcedure(pp.ID)
	return nil
}

func (pp *PatientProcedure) Create() error {
	pp.ID = uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	pp.CreatedAt = now
	pp.UpdatedAt = now
	if pp.Status == "" {
		pp.Status = "planned"
	}
	_, err := DB.Exec(`INSERT INTO patient_procedures (id, patient_id, procedure_id, employee_id, status,
		referred_by_patient_id, referred_by_external, referral_commission_rate, notes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		pp.ID, pp.PatientID, pp.ProcedureID, pp.EmployeeID, pp.Status,
		pp.ReferredByPatientID, pp.ReferredByExternal, pp.ReferralCommissionRate, pp.Notes,
		pp.CreatedAt, pp.UpdatedAt)
	if err != nil {
		return err
	}
	// Auto-create sessions based on procedure_sessions template
	procSessions, _ := DB.Query(`SELECT id, session_number FROM procedure_sessions WHERE procedure_id = ? ORDER BY session_number`, pp.ProcedureID)
	if procSessions != nil {
		defer procSessions.Close()
		for procSessions.Next() {
			var psID string
			var sessionNum int
			if err := procSessions.Scan(&psID, &sessionNum); err != nil {
				continue
			}
			sessID := uuid.New().String()
			DB.Exec(`INSERT INTO patient_procedure_sessions (id, patient_procedure_id, procedure_session_id, session_number, status, created_at, updated_at)
				VALUES (?,?,?,?,?,?,?)`, sessID, pp.ID, psID, sessionNum, "pending", now, now)
		}
	}
	return pp.GetByID(pp.ID)
}

func (pp *PatientProcedure) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"employeeId": "employee_id", "status": "status",
		"referredByPatientId": "referred_by_patient_id", "referredByExternal": "referred_by_external",
		"referralCommissionRate": "referral_commission_rate", "notes": "notes",
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
	args = append(args, time.Now().Format(time.RFC3339))
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

// Sessions

func (s *PatientProcedureSession) GetByPatientProcedure(patientProcedureID string) ([]PatientProcedureSession, error) {
	rows, err := DB.Query(`SELECT id, patient_procedure_id, procedure_session_id, appointment_id,
		status, session_number, scheduled_date, completed_date, notes, created_at, updated_at
		FROM patient_procedure_sessions WHERE patient_procedure_id = ? ORDER BY session_number`, patientProcedureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PatientProcedureSession
	for rows.Next() {
		var sess PatientProcedureSession
		if err := rows.Scan(&sess.ID, &sess.PatientProcedureID, &sess.ProcedureSessionID, &sess.AppointmentID,
			&sess.Status, &sess.SessionNumber, &sess.ScheduledDate, &sess.CompletedDate, &sess.Notes, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, sess)
	}
	return items, rows.Err()
}

func (s *PatientProcedureSession) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, patient_procedure_id, procedure_session_id, appointment_id,
		status, session_number, scheduled_date, completed_date, notes, created_at, updated_at
		FROM patient_procedure_sessions WHERE id = ?`, id).
		Scan(&s.ID, &s.PatientProcedureID, &s.ProcedureSessionID, &s.AppointmentID,
			&s.Status, &s.SessionNumber, &s.ScheduledDate, &s.CompletedDate, &s.Notes, &s.CreatedAt, &s.UpdatedAt)
	return err
}

func (s *PatientProcedureSession) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"appointmentId": "appointment_id", "status": "status",
		"scheduledDate": "scheduled_date", "completedDate": "completed_date", "notes": "notes",
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
		return s.GetByID(s.ID)
	}
	setClauses += ", updated_at = ?"
	args = append(args, time.Now().Format(time.RFC3339))
	args = append(args, s.ID)
	_, err := DB.Exec("UPDATE patient_procedure_sessions SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return s.GetByID(s.ID)
}
