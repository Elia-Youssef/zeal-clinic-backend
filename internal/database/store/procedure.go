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
	Type     *ProcedureType     `json:"type,omitempty"`
	Category *ProcedureCategory `json:"category,omitempty"`
}

func (p *Procedure) IsValid() error {
	if msg := validation.Required(p.Name, "Name"); msg != "" {
		return validation.Errors{"name": msg}
	}
	return nil
}

// procedureSelect joins the active price row so reads expose Price even though
// it's stored in procedure_prices.
const procedureSelect = `SELECT p.id, p.name, p.type_id, p.category_id,
		COALESCE(pp.price, 0), p.price_note, p.is_active, p.remarks, p.includes,
		p.created_at, p.updated_at
	FROM procedures p
	LEFT JOIN procedure_prices pp ON pp.procedure_id = p.id AND pp.is_active = 1`

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
	if fc, fa := params.FilterClause("name", "remarks", "includes", "price_note"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM procedures"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := procedureSelect + where + ` ORDER BY p.name` + params.PaginationClause()
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
	}
	return total, nil
}

func GetProcedureDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("p.name", "c.name", "pc.name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT p.id, p.name, COALESCE(c.name, ''), COALESCE(pc.name, '')
		FROM procedures p
		LEFT JOIN procedure_categories c ON c.id = p.category_id
		LEFT JOIN procedure_categories pc ON pc.id = c.parent_id` + where +
		` ORDER BY p.name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DropdownItem
	for rows.Next() {
		var item DropdownItem
		var catName, parentName string
		if err := rows.Scan(&item.ID, &item.Name, &catName, &parentName); err != nil {
			continue
		}
		if parentName != "" {
			item.Name = parentName + ", " + catName + ", " + item.Name
		} else if catName != "" {
			item.Name = catName + ", " + item.Name
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (p *Procedure) GetByID(id string) error {
	err := p.ScanRow(RDB.QueryRow(procedureSelect+` WHERE p.id = ?`, id))
	if err != nil {
		return err
	}
	p.loadRelations()
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
	_, err = tx.Exec(`INSERT INTO procedures (id, name, type_id, category_id, price_note, is_active, remarks, includes, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.TypeID, p.CategoryID, p.PriceNote,
		BoolToInt(p.IsActive), p.Remarks, p.Includes, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO procedure_prices (id, procedure_id, price, is_active, created_at)
		VALUES (?, ?, ?, 1, ?)`,
		uuid.Must(uuid.NewV7()).String(), p.ID, p.Price, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Procedure) Update(updates map[string]any) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := DateNow()

	// Price changes are written to procedure_prices: deactivate previous active
	// rows and insert a new active row. The procedures row itself doesn't
	// store the price.
	if val, ok := updates["price"]; ok {
		var price float64
		switch v := val.(type) {
		case float64:
			price = v
		case float32:
			price = float64(v)
		case int:
			price = float64(v)
		case int64:
			price = float64(v)
		}
		var current float64
		_ = tx.QueryRow(`SELECT price FROM procedure_prices WHERE procedure_id = ? AND is_active = 1`, p.ID).Scan(&current)
		if current != price {
			if _, err := tx.Exec(`UPDATE procedure_prices SET is_active = 0 WHERE procedure_id = ? AND is_active = 1`, p.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO procedure_prices (id, procedure_id, price, is_active, created_at)
				VALUES (?, ?, ?, 1, ?)`,
				uuid.Must(uuid.NewV7()).String(), p.ID, price, now); err != nil {
				return err
			}
		}
		delete(updates, "price")
	}

	cols := map[string]string{
		"name": "name", "typeId": "type_id", "categoryId": "category_id",
		"priceNote": "price_note",
		"isActive":  "is_active", "remarks": "remarks", "includes": "includes",
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
		args = append(args, now)
		args = append(args, p.ID)
		if _, err := tx.Exec("UPDATE procedures SET "+setClauses+" WHERE id = ?", args...); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
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
