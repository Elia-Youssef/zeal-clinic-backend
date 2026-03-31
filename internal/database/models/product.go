package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Product struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CategoryID   string  `json:"categoryId"`
	Quantity     int     `json:"quantity"`
	MinThreshold int     `json:"minThreshold"`
	UnitPrice    float64 `json:"unitPrice"`
	CreatedAt    Date    `json:"createdAt"`
	// Nested
	AllergyConflicts []ProductAllergyConflict `json:"allergyConflicts,omitempty"`
	Invoices         []Invoice                `json:"invoices,omitempty"`
}

func (p *Product) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(p.Name, "Name"); msg != "" {
		e["name"] = msg
	}
	if msg := validation.Positive(p.UnitPrice, "Unit price"); msg != "" {
		e["unitPrice"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const productColumnsNoId = `name, category_id, quantity, min_threshold, unit_price, created_at`
const productColumns = `id, ` + productColumnsNoId

type ProductList []Product

func (m *Product) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Product row")
	}
	return row.Scan(&m.ID, &m.Name, &m.CategoryID, &m.Quantity, &m.MinThreshold, &m.UnitPrice, &m.CreatedAt)
}

func (l *ProductList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Product rows")
	}
	*l = ProductList{}
	for rows.Next() {
		var item Product
		err := rows.Scan(&item.ID, &item.Name, &item.CategoryID, &item.Quantity, &item.MinThreshold, &item.UnitPrice, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (p *Product) GetAll() (ProductList, error) {
	rows, err := DB.Query(`SELECT ` + productColumns + ` FROM products ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list ProductList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, rows.Err()
}

func (p *Product) GetByID(id string) error {
	err := p.ScanRow(DB.QueryRow(`SELECT `+productColumns+` FROM products WHERE id = ?`, id))
	if err != nil {
		return err
	}
	p.AllergyConflicts, _ = (&ProductAllergyConflict{}).GetByProduct(p.ID)
	p.Invoices, _ = (&Invoice{}).GetByItem(p.ID, "product")
	return nil
}

func (p *Product) Create() error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	p.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO products (`+productColumns+`) VALUES (?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.CategoryID, p.Quantity, p.MinThreshold, p.UnitPrice, p.CreatedAt)
	return err
}

func (p *Product) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "categoryId": "category_id", "quantity": "quantity",
		"minThreshold": "min_threshold", "unitPrice": "unit_price",
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
		return p.GetByID(p.ID)
	}

	args = append(args, p.ID)
	_, err := DB.Exec("UPDATE products SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return p.GetByID(p.ID)
}

func (p *Product) AdjustQuantity(delta int, tx *sql.Tx) error {
	_, err := tx.Exec(`UPDATE products SET quantity = quantity + ? WHERE id = ?`, delta, p.ID)
	return err
}

func (p *Product) Delete() error {
	res, err := DB.Exec("DELETE FROM products WHERE id = ?", p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
