package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Supplier struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Contact   string `json:"contact"`
	Email     string `json:"email"`
	Address   string `json:"address"`
	Notes     string `json:"notes"`
	CreatedAt Date   `json:"createdAt"`
	UpdatedAt Date   `json:"updatedAt"`
	// Nested
	Balance []Balance `json:"balance,omitempty"`
}

func (s *Supplier) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(s.Name, "Name"); msg != "" {
		e["name"] = msg
	}
	if msg := validation.Phone(s.Contact); msg != "" {
		e["contact"] = msg
	}
	if msg := validation.Email(s.Email); msg != "" {
		e["email"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const supplierColumnsNoId = `name, contact, email, address, notes, created_at, updated_at`
const supplierColumns = `id, ` + supplierColumnsNoId

type SupplierList []Supplier

func (s *Supplier) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Supplier row")
	}
	return row.Scan(&s.ID, &s.Name, &s.Contact, &s.Email, &s.Address, &s.Notes, &s.CreatedAt, &s.UpdatedAt)
}

func (l *SupplierList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Supplier rows")
	}
	*l = SupplierList{}
	for rows.Next() {
		var item Supplier
		err := rows.Scan(&item.ID, &item.Name, &item.Contact, &item.Email, &item.Address, &item.Notes, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (l *SupplierList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name", "contact", "email", "address"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM suppliers"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + supplierColumns + ` FROM suppliers` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := l.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

func GetSupplierDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM suppliers` + where + ` ORDER BY name` + params.PaginationClause()
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

func (s *Supplier) GetByID(id string) error {
	err := s.ScanRow(RDB.QueryRow(`SELECT `+supplierColumns+` FROM suppliers WHERE id = ?`, id))
	if err != nil {
		return err
	}
	s.Balance, _ = (&Balance{}).GetByEntityID("supplier", s.ID)
	return nil
}

func (s *Supplier) Create() error {
	s.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	s.CreatedAt = now
	s.UpdatedAt = now

	_, err := DB.Exec(`INSERT INTO suppliers (`+supplierColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
		s.ID, s.Name, s.Contact, s.Email, s.Address, s.Notes, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return err
	}

	// Create a balance for the supplier in each currency
	var currencies CurrencyList
	if _, err := currencies.GetAll(ListParams{}); err == nil {
		for _, cur := range currencies {
			bal := Balance{
				EntityType: "supplier",
				EntityID:   &s.ID,
				EntityName: s.Name,
				CurrencyID: cur.ID,
			}
			bal.GetOrCreate()
		}
	}

	return nil
}

func (s *Supplier) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "contact": "contact", "email": "email",
		"address": "address", "notes": "notes",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			args = append(args, val)
		}
	}
	if setClauses == "" {
		return s.GetByID(s.ID)
	}

	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, s.ID)
	if _, err := DB.Exec("UPDATE suppliers SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}

	return s.GetByID(s.ID)
}

func (s *Supplier) Delete() error {
	res, err := DB.Exec("DELETE FROM suppliers WHERE id = ?", s.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
