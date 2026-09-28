package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Holiday struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartDate Date   `json:"startDate"`
	EndDate   Date   `json:"endDate"`
	Notes     string `json:"notes"`
	CreatedBy string `json:"createdBy"`
	CreatedAt Date   `json:"createdAt"`
	UpdatedAt Date   `json:"updatedAt"`
}

const holidayColumnsNoId = `name, start_date, end_date, notes, created_by, created_at, updated_at`
const holidayColumns = `id, ` + holidayColumnsNoId

type HolidayList []Holiday

func (h *Holiday) IsValid() error {
	if h.Name == "" {
		return fmt.Errorf("%w: Name is required", ErrValidation)
	}
	// Checked on the values as sent, before anything normalizes them: an empty
	// date would otherwise become "0001-01-01" and slip through.
	if h.StartDate.IsZero() || h.EndDate.IsZero() {
		return fmt.Errorf("%w: Start and end dates are required", ErrValidation)
	}
	if validation.Date(h.StartDate.DateOnly()) != "" {
		return fmt.Errorf("%w: Start date must be a valid date", ErrValidation)
	}
	if validation.Date(h.EndDate.DateOnly()) != "" {
		return fmt.Errorf("%w: End date must be a valid date", ErrValidation)
	}
	if h.EndDate.Before(h.StartDate) {
		return fmt.Errorf("%w: End date must be on or after start date", ErrValidation)
	}
	return nil
}

func (h *Holiday) scanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Holiday row")
	}
	return row.Scan(&h.ID, &h.Name, &h.StartDate, &h.EndDate, &h.Notes, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt)
}

func (l *HolidayList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name", "notes"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM holidays"+where, args...).Scan(&total); err != nil {
		return 0, err
	}
	order := params.OrderClause(map[string]string{
		"name":      "name",
		"startDate": "start_date",
		"endDate":   "end_date",
		"createdAt": "created_at",
		"updatedAt": "updated_at",
	}, "start_date DESC")
	rows, err := RDB.Query(`SELECT `+holidayColumns+` FROM holidays`+where+order+params.PaginationClause(), args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	*l = HolidayList{}
	for rows.Next() {
		var item Holiday
		if err := rows.Scan(&item.ID, &item.Name, &item.StartDate, &item.EndDate, &item.Notes, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return total, rows.Err()
}

// HolidaysOverlappingRange returns holidays whose date range intersects
// [start, end]. Pass the same date for both bounds to query a single day.
func HolidaysOverlappingRange(start, end Date) (HolidayList, error) {
	rows, err := RDB.Query(`SELECT `+holidayColumns+` FROM holidays
		WHERE end_date >= ? AND start_date <= ?
		ORDER BY start_date`, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items HolidayList
	for rows.Next() {
		var h Holiday
		if err := rows.Scan(&h.ID, &h.Name, &h.StartDate, &h.EndDate, &h.Notes, &h.CreatedBy, &h.CreatedAt, &h.UpdatedAt); err != nil {
			continue
		}
		items = append(items, h)
	}
	return items, rows.Err()
}

func (h *Holiday) GetByID(id string) error {
	return h.scanRow(RDB.QueryRow(`SELECT `+holidayColumns+` FROM holidays WHERE id = ?`, id))
}

func (h *Holiday) Create() error {
	if err := h.IsValid(); err != nil {
		return err
	}
	h.StartDate = Date(h.StartDate.DateOnly())
	h.EndDate = Date(h.EndDate.DateOnly())
	now := DateNow()
	h.ID = uuid.Must(uuid.NewV7()).String()
	h.CreatedAt = now
	h.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO holidays (`+holidayColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
		h.ID, h.Name, h.StartDate, h.EndDate, h.Notes, h.CreatedBy, h.CreatedAt, h.UpdatedAt)
	return err
}

func (h *Holiday) Update(updates map[string]any) error {
	var current Holiday
	current.ID = h.ID
	if err := current.scanRow(RDB.QueryRow(`SELECT `+holidayColumns+` FROM holidays WHERE id = ?`, h.ID)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	next := current
	if v, ok := stringUpdate(updates, "name"); ok {
		next.Name = v
	}
	if v, ok := dateUpdate(updates, "startDate"); ok {
		next.StartDate = v
	}
	if v, ok := dateUpdate(updates, "endDate"); ok {
		next.EndDate = v
	}
	if v, ok := stringUpdate(updates, "notes"); ok {
		next.Notes = v
	}
	if err := next.IsValid(); err != nil {
		return err
	}
	next.StartDate = Date(next.StartDate.DateOnly())
	next.EndDate = Date(next.EndDate.DateOnly())
	now := DateNow()
	if _, err := DB.Exec(`UPDATE holidays SET name = ?, start_date = ?, end_date = ?, notes = ?, updated_at = ? WHERE id = ?`,
		next.Name, next.StartDate, next.EndDate, next.Notes, now, next.ID); err != nil {
		return err
	}
	next.UpdatedAt = now
	*h = next
	return nil
}

func (h *Holiday) Delete() error {
	res, err := DB.Exec(`DELETE FROM holidays WHERE id = ?`, h.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
