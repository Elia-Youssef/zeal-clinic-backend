package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Procedure struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	TypeID     string  `json:"typeId"`
	CategoryID string  `json:"categoryId"`
	Price      float64 `json:"price"`
	PriceNote  string  `json:"priceNote"`
	IsActive   bool    `json:"isActive"`
	Remarks    string  `json:"remarks"`
	Includes   string  `json:"includes"`
	CreatedAt  Date    `json:"createdAt"`
	UpdatedAt  Date    `json:"updatedAt"`
	// Nested
	Type              *ProcedureType               `json:"type,omitempty"`
	Category          *ProcedureCategory           `json:"category,omitempty"`
	Sessions          ProcedureSessionList         `json:"sessions,omitempty"`
	AllergyConflicts  ProcedureAllergyConflictList `json:"allergyConflicts,omitempty"`
	PatientProcedures PatientProcedureList         `json:"patientProcedures,omitempty"`
}

func (p *Procedure) IsValid() error {
	if msg := validation.Required(p.Name, "Name"); msg != "" {
		return validation.Errors{"name": msg}
	}
	return nil
}

const procedureColumnsNoId = `name, type_id, category_id, price, price_note, is_active, remarks, includes, created_at, updated_at`
const procedureColumns = `id, ` + procedureColumnsNoId

type ProcedureList []Procedure

func (m *Procedure) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Procedure row")
	}
	var isActive int
	err := row.Scan(&m.ID, &m.Name, &m.TypeID, &m.CategoryID, &m.Price, &m.PriceNote,
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
		err := rows.Scan(&item.ID, &item.Name, &item.TypeID, &item.CategoryID, &item.Price, &item.PriceNote,
			&isActive, &item.Remarks, &item.Includes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		item.IsActive = isActive == 1
		*l = append(*l, item)
	}
	return nil
}

func (p *Procedure) loadRelations() {
	if p.TypeID != "" {
		var pt ProcedureType
		if err := pt.GetByID(p.TypeID); err == nil {
			p.Type = &pt
		}
	}
	if p.CategoryID != "" {
		var cat ProcedureCategory
		if err := cat.GetByID(p.CategoryID); err == nil {
			p.Category = &cat
		}
	}
}

func (p *ProcedureList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM procedures"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + procedureColumns + ` FROM procedures` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}

	if err := p.ScanRows(rows); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for i := range *p {
		(*p)[i].loadRelations()
		(*p)[i].Sessions.GetByProcedure((*p)[i].ID)
	}
	return total, nil
}

type ProcedureDropdownItem struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Sessions []DropdownItem `json:"sessions"`
}

func GetProcedureDropdown(params ListParams) ([]ProcedureDropdownItem, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM procedures` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	// Collect all items first so the rows are closed before we query
	// sessions. Holding rows open while calling another Query deadlocks
	// with MaxOpenConns(1).
	var items []ProcedureDropdownItem
	for rows.Next() {
		var item ProcedureDropdownItem
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			continue
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for i := range items {
		sessions, err := GetProcedureSessionDropdown(items[i].ID, ListParams{})
		if err != nil {
			continue
		}
		if sessions == nil {
			sessions = []DropdownItem{}
		}
		items[i].Sessions = sessions
	}
	return items, nil
}

func (p *Procedure) GetByID(id string) error {
	err := p.ScanRow(RDB.QueryRow(`SELECT `+procedureColumns+` FROM procedures WHERE id = ?`, id))
	if err != nil {
		return err
	}
	p.loadRelations()
	p.Sessions.GetByProcedure(p.ID)
	p.PatientProcedures.GetByProcedure(p.ID)
	return nil
}

func (p *Procedure) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	p.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err = tx.Exec(`INSERT INTO procedures (`+procedureColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.TypeID, p.CategoryID, p.Price, p.PriceNote,
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
		if _, err := tx.Exec(`INSERT INTO procedure_sessions (`+procedureSessionColumns+`) VALUES (?,?,?,?,?,?,?)`,
			p.Sessions[i].ID, p.Sessions[i].ProcedureID, p.Sessions[i].SessionNumber,
			p.Sessions[i].Name, p.Sessions[i].Description,
			p.Sessions[i].Price, p.Sessions[i].CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *Procedure) Update(updates map[string]any) error {
	cols := map[string]string{
		"name": "name", "typeId": "type_id", "categoryId": "category_id",
		"price": "price", "priceNote": "price_note",
		"isActive": "is_active", "remarks": "remarks", "includes": "includes",
	}
	setClauses := ""
	var args []any
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
		return ErrNotFound
	}
	return nil
}
