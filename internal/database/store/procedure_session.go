package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type ProcedureSession struct {
	ID            string  `json:"id"`
	ProcedureID   string  `json:"procedureId"`
	SessionNumber int     `json:"sessionNumber"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Price         float64 `json:"price"`
	CreatedAt     Date    `json:"createdAt"`
}

func (s *ProcedureSession) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(s.Name, "Name"); msg != "" {
		e["name"] = msg
	}
	if msg := validation.Positive(float64(s.SessionNumber), "Session number"); msg != "" {
		e["sessionNumber"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const procedureSessionColumnsNoId = `procedure_id, session_number, name, description, price, created_at`
const procedureSessionColumns = `id, ` + procedureSessionColumnsNoId

type ProcedureSessionList []ProcedureSession

func (m *ProcedureSession) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ProcedureSession row")
	}
	return row.Scan(&m.ID, &m.ProcedureID, &m.SessionNumber, &m.Name, &m.Description, &m.Price, &m.CreatedAt)
}

func (l *ProcedureSessionList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ProcedureSession rows")
	}
	*l = ProcedureSessionList{}
	for rows.Next() {
		var item ProcedureSession
		err := rows.Scan(&item.ID, &item.ProcedureID, &item.SessionNumber, &item.Name, &item.Description, &item.Price, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (s *ProcedureSessionList) GetByProcedure(procedureID string) error {
	rows, err := RDB.Query(`SELECT `+procedureSessionColumns+` FROM procedure_sessions WHERE procedure_id = ? ORDER BY session_number`, procedureID)
	if err != nil {
		return err
	}
	defer rows.Close()

	return s.ScanRows(rows)
}

func GetProcedureSessionDropdown(procedureID string, params ListParams) ([]DropdownItem, error) {
	where := " WHERE procedure_id = ?"
	args := []any{procedureID}
	if fc, fa := params.FilterClause("name"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}
	query := `SELECT id, name FROM procedure_sessions` + where + ` ORDER BY session_number` + params.PaginationClause()
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

func (s *ProcedureSession) Create() error {
	s.ID = uuid.Must(uuid.NewV7()).String()
	s.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO procedure_sessions (`+procedureSessionColumns+`) VALUES (?,?,?,?,?,?,?)`,
		s.ID, s.ProcedureID, s.SessionNumber, s.Name, s.Description, s.Price, s.CreatedAt)
	return err
}

func (s *ProcedureSession) Delete() error {
	res, err := DB.Exec("DELETE FROM procedure_sessions WHERE id = ?", s.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
