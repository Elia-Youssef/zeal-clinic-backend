package models

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

func (s *ProcedureSession) GetByProcedure(procedureID string) ([]ProcedureSession, error) {
	rows, err := DB.Query(`SELECT `+procedureSessionColumns+` FROM procedure_sessions WHERE procedure_id = ? ORDER BY session_number`, procedureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list ProcedureSessionList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
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
		return sql.ErrNoRows
	}
	return nil
}
