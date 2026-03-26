package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Procedure struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	ProcedureType   string             `json:"procedureType"`
	Category        string             `json:"category"`
	Subcategory     string             `json:"subcategory"`
	DurationMinutes int                `json:"durationMinutes"`
	CommissionRate  float64            `json:"commissionRate"`
	CommissionType  string             `json:"commissionType"`
	IsActive        bool               `json:"isActive"`
	Remarks         string             `json:"remarks"`
	CreatedAt       string             `json:"createdAt"`
	UpdatedAt       string             `json:"updatedAt"`
	// Nested
	Sessions         []ProcedureSession         `json:"sessions,omitempty"`
	AllergyConflicts []ProcedureAllergyConflict `json:"allergyConflicts,omitempty"`
}

type ProcedureSession struct {
	ID              string  `json:"id"`
	ProcedureID     string  `json:"procedureId"`
	SessionNumber   int     `json:"sessionNumber"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	DurationMinutes int     `json:"durationMinutes"`
	Price           float64 `json:"price"`
	Currency        string  `json:"currency"`
	CreatedAt       string  `json:"createdAt"`
}

func (p *Procedure) GetAll() ([]Procedure, error) {
	rows, err := DB.Query(`SELECT id, name, procedure_type, category, subcategory, duration_minutes,
		commission_rate, commission_type, is_active, remarks, created_at, updated_at
		FROM procedures ORDER BY category, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Procedure
	for rows.Next() {
		var proc Procedure
		var isActive int
		if err := rows.Scan(&proc.ID, &proc.Name, &proc.ProcedureType, &proc.Category, &proc.Subcategory, &proc.DurationMinutes,
			&proc.CommissionRate, &proc.CommissionType, &isActive, &proc.Remarks, &proc.CreatedAt, &proc.UpdatedAt); err != nil {
			return nil, err
		}
		proc.IsActive = isActive == 1
		sessions, err := (&ProcedureSession{}).GetByProcedure(proc.ID)
		if err != nil {
			return nil, err
		}
		proc.Sessions = sessions
		items = append(items, proc)
	}
	return items, rows.Err()
}

func (p *Procedure) GetByID(id string) error {
	var isActive int
	err := DB.QueryRow(`SELECT id, name, procedure_type, category, subcategory, duration_minutes,
		commission_rate, commission_type, is_active, remarks, created_at, updated_at
		FROM procedures WHERE id = ?`, id).
		Scan(&p.ID, &p.Name, &p.ProcedureType, &p.Category, &p.Subcategory, &p.DurationMinutes,
			&p.CommissionRate, &p.CommissionType, &isActive, &p.Remarks, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return err
	}
	p.IsActive = isActive == 1
	p.Sessions, _ = (&ProcedureSession{}).GetByProcedure(p.ID)
	return nil
}

func (p *Procedure) Create() error {
	p.ID = uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO procedures (id, name, procedure_type, category, subcategory, duration_minutes,
		commission_rate, commission_type, is_active, remarks, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.ProcedureType, p.Category, p.Subcategory, p.DurationMinutes,
		p.CommissionRate, p.CommissionType, BoolToInt(p.IsActive), p.Remarks, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return err
	}
	for i := range p.Sessions {
		p.Sessions[i].ProcedureID = p.ID
		p.Sessions[i].ID = uuid.New().String()
		p.Sessions[i].CreatedAt = now
		if p.Sessions[i].SessionNumber == 0 {
			p.Sessions[i].SessionNumber = i + 1
		}
		_, err := DB.Exec(`INSERT INTO procedure_sessions (id, procedure_id, session_number, name, description, duration_minutes, price, currency, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			p.Sessions[i].ID, p.Sessions[i].ProcedureID, p.Sessions[i].SessionNumber,
			p.Sessions[i].Name, p.Sessions[i].Description, p.Sessions[i].DurationMinutes,
			p.Sessions[i].Price, p.Sessions[i].Currency, p.Sessions[i].CreatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Procedure) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "procedureType": "procedure_type", "category": "category",
		"subcategory": "subcategory", "durationMinutes": "duration_minutes",
		"commissionRate": "commission_rate", "commissionType": "commission_type",
		"isActive": "is_active", "remarks": "remarks",
	}
	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if dbCol == "is_active" {
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
	if setClauses != "" {
		setClauses += ", updated_at = ?"
		args = append(args, time.Now().Format(time.RFC3339))
		args = append(args, p.ID)
		_, err := DB.Exec("UPDATE procedures SET "+setClauses+" WHERE id = ?", args...)
		if err != nil {
			return err
		}
	}
	return p.GetByID(p.ID)
}

func (p *Procedure) Delete() error {
	res, err := DB.Exec("DELETE FROM procedures WHERE id = ?", p.ID)
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

func (s *ProcedureSession) GetByProcedure(procedureID string) ([]ProcedureSession, error) {
	rows, err := DB.Query(`SELECT id, procedure_id, session_number, name, description, duration_minutes, price, currency, created_at
		FROM procedure_sessions WHERE procedure_id = ? ORDER BY session_number`, procedureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ProcedureSession
	for rows.Next() {
		var sess ProcedureSession
		if err := rows.Scan(&sess.ID, &sess.ProcedureID, &sess.SessionNumber, &sess.Name, &sess.Description, &sess.DurationMinutes, &sess.Price, &sess.Currency, &sess.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, sess)
	}
	return items, rows.Err()
}

func (s *ProcedureSession) Create() error {
	s.ID = uuid.New().String()
	s.CreatedAt = time.Now().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO procedure_sessions (id, procedure_id, session_number, name, description, duration_minutes, price, currency, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		s.ID, s.ProcedureID, s.SessionNumber, s.Name, s.Description, s.DurationMinutes, s.Price, s.Currency, s.CreatedAt)
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
