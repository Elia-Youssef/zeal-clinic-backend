package models

import (
	"database/sql"

	"github.com/google/uuid"
)

type AppointmentPhoto struct {
	ID            string `json:"id"`
	AppointmentID string `json:"appointmentId"`
	PhotoType     string `json:"photoType"`
	FilePath      string `json:"filePath"`
	Caption       string `json:"caption"`
	SortOrder     int    `json:"sortOrder"`
	CreatedAt     string `json:"createdAt"`
}

func (p *AppointmentPhoto) GetByAppointmentID(appointmentID string) ([]AppointmentPhoto, error) {
	rows, err := DB.Query(`SELECT id, appointment_id, photo_type, file_path, caption, sort_order, created_at
		FROM appointment_photos WHERE appointment_id = ? ORDER BY sort_order`, appointmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []AppointmentPhoto
	for rows.Next() {
		var photo AppointmentPhoto
		if err := rows.Scan(&photo.ID, &photo.AppointmentID, &photo.PhotoType, &photo.FilePath, &photo.Caption, &photo.SortOrder, &photo.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, photo)
	}
	return items, rows.Err()
}

func (p *AppointmentPhoto) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, appointment_id, photo_type, file_path, caption, sort_order, created_at
		FROM appointment_photos WHERE id = ?`, id).
		Scan(&p.ID, &p.AppointmentID, &p.PhotoType, &p.FilePath, &p.Caption, &p.SortOrder, &p.CreatedAt)
	return err
}

func (p *AppointmentPhoto) Create() error {
	p.ID = uuid.New().String()
	_, err := DB.Exec(`INSERT INTO appointment_photos (id, appointment_id, photo_type, file_path, caption, sort_order, created_at)
		VALUES (?,?,?,?,?,?,?)`,
		p.ID, p.AppointmentID, p.PhotoType, p.FilePath, p.Caption, p.SortOrder, p.CreatedAt)
	return err
}

func (p *AppointmentPhoto) Delete() error {
	res, err := DB.Exec("DELETE FROM appointment_photos WHERE id = ?", p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
