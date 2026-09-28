package store

import (
	"github.com/google/uuid"
)

const appointmentProcedureColumnsNoId = `patient_id, procedure_id, appointment_id, assigned_to_id, notes, created_at, updated_at`
const appointmentProcedureColumns = `id, ` + appointmentProcedureColumnsNoId

type AppointmentProcedure struct {
	ID            string `json:"id"`
	PatientID     string `json:"patientId"`
	ProcedureID   string `json:"procedureId"`
	AppointmentID string `json:"appointmentId"`
	AssignedToID  string `json:"assignedToId"`
	Notes         string `json:"notes"`
	CreatedAt     Date   `json:"createdAt"`
	UpdatedAt     Date   `json:"updatedAt"`
	// Joined fields
	ProcedureName  string `json:"procedureName,omitempty"`
	AssignedToName string `json:"assignedToName,omitempty"`
}

type AppointmentProcedureList []AppointmentProcedure

// create inserts the appointment_procedure inside the supplied DBTX so it can
// run as part of an existing transaction (e.g. Appointment.Create / Update),
// whose exit carries the constraint translation for the whole write.
func (ap *AppointmentProcedure) create(db DBTX) error {
	ap.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	ap.CreatedAt = now
	ap.UpdatedAt = now
	_, err := db.Exec(`INSERT INTO appointment_procedures (`+appointmentProcedureColumns+`)
		VALUES (?,?,?,?,?,?,?,?)`,
		ap.ID, ap.PatientID, ap.ProcedureID, ap.AppointmentID,
		nullableID(ap.AssignedToID), ap.Notes, ap.CreatedAt, ap.UpdatedAt)
	return err
}
