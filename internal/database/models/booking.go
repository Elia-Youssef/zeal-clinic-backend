package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Booking struct {
	ID              string `json:"id"`
	ClientName      string `json:"clientName"`
	ClientPhone     string `json:"clientPhone"`
	ClientEmail     string `json:"clientEmail"`
	IsNewClient     bool   `json:"isNewClient"`
	ReferralSource  string `json:"referralSource"`
	ServiceCategory string `json:"serviceCategory"`
	ServiceName     string `json:"serviceName"`
	PreferredDate   Date   `json:"preferredDate"`
	PreferredTime   string `json:"preferredTime"`
	DurationMinutes int    `json:"durationMinutes"`
	Status          string `json:"status"`
	AppointmentID   string `json:"appointmentId"`
	PatientID       string `json:"patientId"`
	RoomID          string `json:"roomId"`
	Notes           string `json:"notes"`
	CreatedAt       Date   `json:"createdAt"`
	UpdatedAt       Date   `json:"updatedAt"`
}

func (b *Booking) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(b.ClientName, "Client name"); msg != "" {
		e["clientName"] = msg
	} else if msg := validation.MinLength(b.ClientName, 2, "Client name"); msg != "" {
		e["clientName"] = msg
	}
	if msg := validation.Required(b.ClientPhone, "Phone"); msg != "" {
		e["clientPhone"] = msg
	} else if msg := validation.Phone(b.ClientPhone); msg != "" {
		e["clientPhone"] = msg
	}
	if msg := validation.Email(b.ClientEmail); msg != "" {
		e["clientEmail"] = msg
	}
	if msg := validation.Required(b.ServiceCategory, "Service category"); msg != "" {
		e["serviceCategory"] = msg
	}
	if msg := validation.Required(b.ServiceName, "Service name"); msg != "" {
		e["serviceName"] = msg
	}
	if msg := validation.Required(string(b.PreferredDate), "Preferred date"); msg != "" {
		e["preferredDate"] = msg
	} else if msg := validation.Date(string(b.PreferredDate)); msg != "" {
		e["preferredDate"] = msg
	}
	if msg := validation.Required(b.PreferredTime, "Preferred time"); msg != "" {
		e["preferredTime"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const bookingColumnsNoId = `client_name, client_phone, client_email, is_new_client, referral_source,
	service_category, service_name, preferred_date, preferred_time, duration_minutes,
	status, appointment_id, patient_id, room_id, notes, created_at, updated_at`
const bookingColumns = `id, ` + bookingColumnsNoId

type BookingList []Booking

func (b *Booking) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil booking row")
	}
	var isNew int
	err := row.Scan(
		&b.ID, &b.ClientName, &b.ClientPhone, &b.ClientEmail, &isNew, &b.ReferralSource,
		&b.ServiceCategory, &b.ServiceName, &b.PreferredDate, &b.PreferredTime, &b.DurationMinutes,
		&b.Status, &b.AppointmentID, &b.PatientID, &b.RoomID, &b.Notes, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return err
	}
	b.IsNewClient = isNew == 1
	return nil
}

func (l *BookingList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil booking rows")
	}
	*l = BookingList{}
	for rows.Next() {
		var item Booking
		var isNew int
		err := rows.Scan(
			&item.ID, &item.ClientName, &item.ClientPhone, &item.ClientEmail, &isNew, &item.ReferralSource,
			&item.ServiceCategory, &item.ServiceName, &item.PreferredDate, &item.PreferredTime, &item.DurationMinutes,
			&item.Status, &item.AppointmentID, &item.PatientID, &item.RoomID, &item.Notes, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			continue
		}
		item.IsNewClient = isNew == 1
		*l = append(*l, item)
	}
	return nil
}

func (b *Booking) GetAll(status, date string, params ListParams) ([]Booking, int, error) {
	where := " WHERE 1=1"
	var args []interface{}

	if status != "" {
		where += " AND status = ?"
		args = append(args, status)
	}
	if date != "" {
		where += " AND preferred_date = ?"
		args = append(args, date)
	}
	if fc, fa := params.FilterClause("client_name", "client_phone", "service_category", "service_name"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM bookings"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + bookingColumns + ` FROM bookings` + where + ` ORDER BY created_at DESC` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list BookingList
	if err := list.ScanRows(rows); err != nil {
		return nil, 0, err
	}
	return list, total, rows.Err()
}

func (b *Booking) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+bookingColumns+` FROM bookings WHERE id = ?`, id)
	return b.ScanRow(row)
}

func (b *Booking) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var activeCount int
	err = tx.QueryRow(`SELECT COUNT(*) FROM bookings WHERE client_phone = ? AND status IN ('pending','confirmed')`, b.ClientPhone).Scan(&activeCount)
	if err != nil {
		return fmt.Errorf("check booking limit: %w", err)
	}
	if activeCount >= 3 {
		return fmt.Errorf("booking limit reached: you already have %d active bookings (maximum 3)", activeCount)
	}

	b.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	b.CreatedAt = now
	b.UpdatedAt = now
	if b.Status == "" {
		b.Status = "pending"
	}
	if b.DurationMinutes == 0 {
		b.DurationMinutes = 60
	}

	_, err = tx.Exec(`INSERT INTO bookings (id, client_name, client_phone, client_email, is_new_client, referral_source,
		service_category, service_name, preferred_date, preferred_time, duration_minutes,
		status, appointment_id, patient_id, room_id, notes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		b.ID, b.ClientName, b.ClientPhone, b.ClientEmail, BoolToInt(b.IsNewClient), b.ReferralSource,
		b.ServiceCategory, b.ServiceName, b.PreferredDate, b.PreferredTime, b.DurationMinutes,
		b.Status, b.AppointmentID, b.PatientID, b.RoomID, b.Notes, b.CreatedAt, b.UpdatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (b *Booking) Confirm(roomID string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var isNew int
	err = tx.QueryRow(`SELECT id, client_name, client_phone, client_email, is_new_client, referral_source,
		service_category, service_name, preferred_date, preferred_time, duration_minutes,
		status, appointment_id, patient_id, room_id, notes, created_at, updated_at
		FROM bookings WHERE id = ?`, b.ID).Scan(
		&b.ID, &b.ClientName, &b.ClientPhone, &b.ClientEmail, &isNew, &b.ReferralSource,
		&b.ServiceCategory, &b.ServiceName, &b.PreferredDate, &b.PreferredTime, &b.DurationMinutes,
		&b.Status, &b.AppointmentID, &b.PatientID, &b.RoomID, &b.Notes, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return err
	}
	b.IsNewClient = isNew == 1

	if b.Status != "pending" {
		return fmt.Errorf("booking is already %s", b.Status)
	}

	var patientID string
	err = tx.QueryRow(`SELECT id FROM patients WHERE contact = ? LIMIT 1`, b.ClientPhone).Scan(&patientID)
	if err == sql.ErrNoRows {
		patientID = uuid.Must(uuid.NewV7()).String()
		now := DateNow()
		_, err = tx.Exec(`INSERT INTO patients (id, first_name, last_name, gender, date_of_birth, contact, email, created_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			patientID, b.ClientName, "", "Female", "2000-01-01", b.ClientPhone, b.ClientEmail, now,
		)
		if err != nil {
			return fmt.Errorf("create patient: %w", err)
		}
	} else if err != nil {
		return err
	}

	aptID := uuid.Must(uuid.NewV7()).String()
	startTime := fmt.Sprintf("%sT%s:00", b.PreferredDate, b.PreferredTime)
	t, err := time.Parse("2006-01-02T15:04:05", startTime)
	if err != nil {
		return fmt.Errorf("parse time: %w", err)
	}
	endTime := t.Add(time.Duration(b.DurationMinutes) * time.Minute).Format("2006-01-02T15:04:05")

	now := DateNow()
	_, err = tx.Exec(`INSERT INTO appointments (id, patient_id, room_id, employee_id, patient_procedure_session_id, start_time, end_time, status, notes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		aptID, patientID, roomID, "", nil, startTime, endTime, "Scheduled",
		fmt.Sprintf("Online booking by %s (%s) — %s", b.ClientName, b.ClientPhone, b.ServiceName),
		now, now,
	)
	if err != nil {
		return fmt.Errorf("create appointment: %w", err)
	}

	_, err = tx.Exec(`UPDATE bookings SET status = 'confirmed', appointment_id = ?, patient_id = ?, room_id = ?, updated_at = ? WHERE id = ?`,
		aptID, patientID, roomID, now, b.ID)
	if err != nil {
		return err
	}

	b.Status = "confirmed"
	b.AppointmentID = aptID
	b.PatientID = patientID
	b.RoomID = roomID
	b.UpdatedAt = now

	return tx.Commit()
}

func (b *Booking) Cancel() error {
	now := DateNow()
	_, err := DB.Exec(`UPDATE bookings SET status = 'cancelled', updated_at = ? WHERE id = ?`, now, b.ID)
	if err != nil {
		return err
	}
	return b.GetByID(b.ID)
}

func (b *Booking) GetBookedSlots(date string) ([]string, error) {
	rows, err := RDB.Query(`SELECT preferred_time FROM bookings WHERE preferred_date = ? AND status IN ('pending','confirmed')`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slots []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		slots = append(slots, t)
	}
	return slots, rows.Err()
}

func (b *Booking) PatientCheckByPhone(phone string) (bool, string, error) {
	var firstName string
	err := RDB.QueryRow(`SELECT first_name FROM patients WHERE contact = ? LIMIT 1`, phone).Scan(&firstName)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, firstName, nil
}
