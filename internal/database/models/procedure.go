package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Procedure struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	ProcedureType string  `json:"procedureType"`
	Category      string  `json:"category"`
	Subcategory   string  `json:"subcategory"`
	Price         float64 `json:"price"`
	PriceNote     string  `json:"priceNote"`
	IsActive      bool    `json:"isActive"`
	Remarks       string  `json:"remarks"`
	Includes      string  `json:"includes"`
	CreatedAt     Date    `json:"createdAt"`
	UpdatedAt     Date    `json:"updatedAt"`
	// Nested
	Sessions          []ProcedureSession         `json:"sessions,omitempty"`
	AllergyConflicts  []ProcedureAllergyConflict `json:"allergyConflicts,omitempty"`
	PatientProcedures []PatientProcedure         `json:"patientProcedures,omitempty"`
}

func (p *Procedure) IsValid() error {
	if msg := validation.Required(p.Name, "Name"); msg != "" {
		return validation.Errors{"name": msg}
	}
	return nil
}

const procedureColumnsNoId = `name, procedure_type, category, subcategory, price, price_note, is_active, remarks, includes, created_at, updated_at`
const procedureColumns = `id, ` + procedureColumnsNoId

type ProcedureList []Procedure

func (m *Procedure) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Procedure row")
	}
	var isActive int
	err := row.Scan(&m.ID, &m.Name, &m.ProcedureType, &m.Category, &m.Subcategory, &m.Price, &m.PriceNote,
		&isActive, &m.Remarks, &m.Includes, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return err
	}
	m.IsActive = isActive == 1
	return nil
}

func (l *ProcedureList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Procedure rows")
	}
	*l = ProcedureList{}
	for rows.Next() {
		var item Procedure
		var isActive int
		err := rows.Scan(&item.ID, &item.Name, &item.ProcedureType, &item.Category, &item.Subcategory, &item.Price, &item.PriceNote,
			&isActive, &item.Remarks, &item.Includes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		item.IsActive = isActive == 1
		*l = append(*l, item)
	}
	return nil
}

func (p *Procedure) GetAll() ([]Procedure, error) {
	rows, err := DB.Query(`SELECT ` + procedureColumns + ` FROM procedures ORDER BY category, name`)
	if err != nil {
		return nil, err
	}

	var list ProcedureList
	if err := list.ScanRows(rows); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range list {
		list[i].Sessions, _ = (&ProcedureSession{}).GetByProcedure(list[i].ID)
	}
	return list, nil
}

func (p *Procedure) GetByID(id string) error {
	err := p.ScanRow(DB.QueryRow(`SELECT `+procedureColumns+` FROM procedures WHERE id = ?`, id))
	if err != nil {
		return err
	}
	p.Sessions, _ = (&ProcedureSession{}).GetByProcedure(p.ID)
	p.PatientProcedures, _ = (&PatientProcedure{}).GetByProcedure(p.ID)
	return nil
}

func (p *Procedure) Create() error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := DB.Exec(`INSERT INTO procedures (`+procedureColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.ProcedureType, p.Category, p.Subcategory, p.Price, p.PriceNote,
		BoolToInt(p.IsActive), p.Remarks, p.Includes, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return err
	}
	for i := range p.Sessions {
		p.Sessions[i].ProcedureID = p.ID
		p.Sessions[i].ID = uuid.Must(uuid.NewV7()).String()
		p.Sessions[i].CreatedAt = now
		if p.Sessions[i].SessionNumber == 0 {
			p.Sessions[i].SessionNumber = i + 1
		}
		_, err := DB.Exec(`INSERT INTO procedure_sessions (`+procedureSessionColumns+`) VALUES (?,?,?,?,?,?,?)`,
			p.Sessions[i].ID, p.Sessions[i].ProcedureID, p.Sessions[i].SessionNumber,
			p.Sessions[i].Name, p.Sessions[i].Description,
			p.Sessions[i].Price, p.Sessions[i].CreatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Procedure) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "procedureType": "procedure_type", "category": "category",
		"subcategory": "subcategory", "price": "price", "priceNote": "price_note",
		"isActive": "is_active", "remarks": "remarks", "includes": "includes",
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
		args = append(args, DateNow())
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
