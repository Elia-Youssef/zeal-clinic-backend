package models

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type AppointmentPhoto struct {
	ID            string `json:"id"`
	AppointmentID string `json:"appointmentId"`
	FilePath      string `json:"filePath"`
	Caption       string `json:"caption"`
	CreatedAt     Date `json:"createdAt"`
}

const appointmentPhotoColumnsNoId = `appointment_id, file_path, caption, created_at`
const appointmentPhotoColumns = `id, ` + appointmentPhotoColumnsNoId

type AppointmentPhotoList []AppointmentPhoto

func (m *AppointmentPhoto) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil AppointmentPhoto row")
	}
	return row.Scan(&m.ID, &m.AppointmentID, &m.FilePath, &m.Caption, &m.CreatedAt)
}

func (l *AppointmentPhotoList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil AppointmentPhoto rows")
	}
	*l = AppointmentPhotoList{}
	for rows.Next() {
		var item AppointmentPhoto
		err := rows.Scan(&item.ID, &item.AppointmentID, &item.FilePath, &item.Caption, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (p *AppointmentPhoto) GetByAppointmentID(appointmentID string) (AppointmentPhotoList, error) {
	rows, err := DB.Query(`SELECT `+appointmentPhotoColumns+` FROM appointment_photos WHERE appointment_id = ? ORDER BY created_at`, appointmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list AppointmentPhotoList
	list.ScanRows(rows)
	return list, nil
}

func (p *AppointmentPhoto) GetByID(id string) error {
	return p.ScanRow(DB.QueryRow(`SELECT `+appointmentPhotoColumns+` FROM appointment_photos WHERE id = ?`, id))
}

func (p *AppointmentPhoto) Create() error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	p.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO appointment_photos (`+appointmentPhotoColumns+`) VALUES (?,?,?,?,?)`,
		p.ID, p.AppointmentID, p.FilePath, p.Caption, p.CreatedAt)
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
