package store

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
	Category *ProductCategory `json:"category,omitempty"`
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

// productSelect joins the active price row so reads expose UnitPrice even
// though it's stored in product_prices.
const productSelect = `SELECT p.id, p.name, p.category_id, p.quantity, p.min_threshold,
		COALESCE(pp.price, 0), p.created_at
	FROM products p
	LEFT JOIN product_prices pp ON pp.product_id = p.id AND pp.is_active = 1`

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

func (p *Product) loadRelations() {
	if p.CategoryID != "" {
		var cat ProductCategory
		if err := cat.GetByID(p.CategoryID); err == nil {
			p.Category = &cat
		}
	}
}

func (p *ProductList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM products"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"name":         "p.name",
		"categoryId":   "p.category_id",
		"quantity":     "p.quantity",
		"minThreshold": "p.min_threshold",
		"unitPrice":    "COALESCE(pp.price, 0)",
		"createdAt":    "p.created_at",
	}, "p.name")
	query := productSelect + where + order + params.PaginationClause()
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

func GetProductDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM products` + where + ` ORDER BY name` + params.PaginationClause()
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

func (p *Product) GetByID(id string) error {
	err := p.ScanRow(RDB.QueryRow(productSelect+` WHERE p.id = ?`, id))
	if err != nil {
		return err
	}
	p.loadRelations()
	return nil
}

func (p *Product) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	p.ID = uuid.Must(uuid.NewV7()).String()
	p.CreatedAt = DateNow()
	if _, err := tx.Exec(`INSERT INTO products (id, name, category_id, quantity, min_threshold, created_at) VALUES (?,?,?,?,?,?)`,
		p.ID, p.Name, p.CategoryID, p.Quantity, p.MinThreshold, p.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO product_prices (id, product_id, price, is_active, created_at)
		VALUES (?, ?, ?, 1, ?)`,
		uuid.Must(uuid.NewV7()).String(), p.ID, p.UnitPrice, p.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Product) Update(updates map[string]any) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Price changes are written to product_prices: deactivate previous active
	// rows and insert a new active row.
	if val, ok := updates["unitPrice"]; ok {
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
		_ = tx.QueryRow(`SELECT price FROM product_prices WHERE product_id = ? AND is_active = 1`, p.ID).Scan(&current)
		if current != price {
			if _, err := tx.Exec(`UPDATE product_prices SET is_active = 0 WHERE product_id = ? AND is_active = 1`, p.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO product_prices (id, product_id, price, is_active, created_at)
				VALUES (?, ?, ?, 1, ?)`,
				uuid.Must(uuid.NewV7()).String(), p.ID, price, DateNow()); err != nil {
				return err
			}
		}
		delete(updates, "unitPrice")
	}

	cols := map[string]string{
		"name": "name", "categoryId": "category_id", "quantity": "quantity",
		"minThreshold": "min_threshold",
	}

	setClauses := ""
	var args []any
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			args = append(args, val)
		}
	}
	if setClauses != "" {
		args = append(args, p.ID)
		if _, err := tx.Exec("UPDATE products SET "+setClauses+" WHERE id = ?", args...); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
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
		return ErrNotFound
	}
	return nil
}
