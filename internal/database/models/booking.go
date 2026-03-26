package models

import (
	"database/sql"
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
	PreferredDate   string `json:"preferredDate"`
	PreferredTime   string `json:"preferredTime"`
	DurationMinutes int    `json:"durationMinutes"`
	Status          string `json:"status"`
	AppointmentID   string `json:"appointmentId"`
	PatientID       string `json:"patientId"`
	RoomID          string `json:"roomId"`
	Notes           string `json:"notes"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

func (b *Booking) GetAll(status, date string) ([]Booking, error) {
	query := `SELECT id, client_name, client_phone, client_email, is_new_client, referral_source,
		service_category, service_name, preferred_date, preferred_time, duration_minutes,
		status, appointment_id, patient_id, room_id, notes, created_at, updated_at
		FROM bookings WHERE 1=1`
	var args []interface{}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	if date != "" {
		query += " AND preferred_date = ?"
		args = append(args, date)
	}
	query += " ORDER BY created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	return bookings, rows.Err()
}

func (b *Booking) GetByID(id string) error {
	row := DB.QueryRow(`SELECT id, client_name, client_phone, client_email, is_new_client, referral_source,
		service_category, service_name, preferred_date, preferred_time, duration_minutes,
		status, appointment_id, patient_id, room_id, notes, created_at, updated_at
		FROM bookings WHERE id = ?`, id)
	result, err := scanBookingRow(row)
	if err != nil {
		return err
	}
	*b = result
	return nil
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

	b.ID = uuid.New().String()
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
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

	notifID := uuid.New().String()
	_, err = tx.Exec(`INSERT INTO notifications (id, type, title, message, booking_id, is_read, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		notifID, "new_booking",
		"New Booking Request",
		fmt.Sprintf("%s booked %s on %s at %s", b.ClientName, b.ServiceName, b.PreferredDate, b.PreferredTime),
		b.ID, 0, now,
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
		var nextNum int
		err = tx.QueryRow(`UPDATE counters SET value = value + 1 WHERE name = 'patient' RETURNING value`).Scan(&nextNum)
		if err != nil {
			return fmt.Errorf("counter: %w", err)
		}
		patientID = fmt.Sprintf("PAT-%03d", nextNum)
		now := time.Now().UTC().Format(time.RFC3339)
		_, err = tx.Exec(`INSERT INTO patients (id, patient_number, first_name, last_name, gender, date_of_birth, contact, email, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			patientID, nextNum, b.ClientName, "", "Female", "2000-01-01", b.ClientPhone, b.ClientEmail, now,
		)
		if err != nil {
			return fmt.Errorf("create patient: %w", err)
		}
	} else if err != nil {
		return err
	}

	aptID := uuid.New().String()
	startTime := fmt.Sprintf("%sT%s:00", b.PreferredDate, b.PreferredTime)
	t, err := time.Parse("2006-01-02T15:04:05", startTime)
	if err != nil {
		return fmt.Errorf("parse time: %w", err)
	}
	endTime := t.Add(time.Duration(b.DurationMinutes) * time.Minute).Format("2006-01-02T15:04:05")

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = tx.Exec(`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, treatment_type, status, notes, approval_status, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		aptID, patientID, roomID, startTime, endTime, "Procedure", "Scheduled",
		fmt.Sprintf("Online booking by %s (%s) — %s", b.ClientName, b.ClientPhone, b.ServiceName),
		"pending", now, now,
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
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = tx.Exec(`UPDATE bookings SET status = 'cancelled', updated_at = ? WHERE id = ?`, now, b.ID)
	if err != nil {
		return err
	}

	if err := b.GetByID(b.ID); err == nil {
		notifID := uuid.New().String()
		tx.Exec(`INSERT INTO notifications (id, type, title, message, booking_id, is_read, created_at)
			VALUES (?,?,?,?,?,?,?)`,
			notifID, "booking_cancelled", "Booking Cancelled",
			fmt.Sprintf("Booking by %s for %s has been cancelled", b.ClientName, b.ServiceName),
			b.ID, 0, now,
		)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return b.GetByID(b.ID)
}

func (b *Booking) GetBookedSlots(date string) ([]string, error) {
	rows, err := DB.Query(`SELECT preferred_time FROM bookings WHERE preferred_date = ? AND status IN ('pending','confirmed')`, date)
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
	err := DB.QueryRow(`SELECT first_name FROM patients WHERE contact = ? LIMIT 1`, phone).Scan(&firstName)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, firstName, nil
}

// scan helpers

type bookingScannable interface {
	Scan(dest ...interface{}) error
}

func scanBookingFields(s bookingScannable) (Booking, error) {
	var b Booking
	var isNew int
	err := s.Scan(
		&b.ID, &b.ClientName, &b.ClientPhone, &b.ClientEmail, &isNew, &b.ReferralSource,
		&b.ServiceCategory, &b.ServiceName, &b.PreferredDate, &b.PreferredTime, &b.DurationMinutes,
		&b.Status, &b.AppointmentID, &b.PatientID, &b.RoomID, &b.Notes, &b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return b, err
	}
	b.IsNewClient = isNew == 1
	return b, nil
}

func scanBooking(rows *sql.Rows) (Booking, error) {
	return scanBookingFields(rows)
}

func scanBookingRow(row *sql.Row) (Booking, error) {
	return scanBookingFields(row)
}
