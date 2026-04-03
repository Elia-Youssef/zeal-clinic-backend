package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Room struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	IsAvailable bool   `json:"isAvailable"`
	CreatedAt   Date `json:"createdAt"`
}

const roomColumnsNoId = `name, type, is_available, created_at`
const roomColumns = `id, ` + roomColumnsNoId

type RoomList []Room

func (m *Room) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Room row")
	}
	var isAvailableRaw int
	err := row.Scan(&m.ID, &m.Name, &m.Type, &isAvailableRaw, &m.CreatedAt)
	if err != nil {
		return err
	}
	m.IsAvailable = isAvailableRaw == 1
	return nil
}

func (l *RoomList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Room rows")
	}
	*l = RoomList{}
	for rows.Next() {
		var item Room
		var isAvailableRaw int
		err := rows.Scan(&item.ID, &item.Name, &item.Type, &isAvailableRaw, &item.CreatedAt)
		if err != nil {
			continue
		}
		item.IsAvailable = isAvailableRaw == 1
		*l = append(*l, item)
	}
	return nil
}

func (r *Room) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(r.Name, "Name"); msg != "" {
		e["name"] = msg
	}
	if msg := validation.Required(r.Type, "Type"); msg != "" {
		e["type"] = msg
	} else if msg := validation.OneOf(r.Type, []string{"Consultation", "Procedure", "General", "Hospital"}, "Type"); msg != "" {
		e["type"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func (r *Room) GetAll(params ListParams) (RoomList, int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name", "type"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM rooms"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + roomColumns + ` FROM rooms` + where + ` ORDER BY id` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list RoomList
	list.ScanRows(rows)
	return list, total, nil
}

func GetRoomDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM rooms` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DropdownItem
	for rows.Next() {
		var item DropdownItem
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Room) GetByID(id string) error {
	return r.ScanRow(RDB.QueryRow(`SELECT `+roomColumns+` FROM rooms WHERE id = ?`, id))
}

func (r *Room) Create() error {
	r.ID = uuid.Must(uuid.NewV7()).String()
	r.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO rooms (`+roomColumns+`) VALUES (?,?,?,?,?)`,
		r.ID, r.Name, r.Type, BoolToInt(r.IsAvailable), r.CreatedAt)
	return err
}

func (r *Room) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "type": "type", "isAvailable": "is_available",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if dbCol == "is_available" {
				if b, ok := val.(bool); ok {
					val = BoolToInt(b)
				}
			}
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			args = append(args, val)
		}
	}
	if setClauses == "" {
		return r.GetByID(r.ID)
	}

	args = append(args, r.ID)
	_, err := DB.Exec("UPDATE rooms SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return r.GetByID(r.ID)
}

func (r *Room) Delete() error {
	res, err := DB.Exec("DELETE FROM rooms WHERE id = ?", r.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
