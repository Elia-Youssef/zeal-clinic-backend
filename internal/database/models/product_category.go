package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type ProductCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    string `json:"parentId"`
	CreatedAt   Date   `json:"createdAt"`
}

func (c *ProductCategory) IsValid() error {
	if msg := validation.Required(c.Name, "Name"); msg != "" {
		return validation.Errors{"name": msg}
	}
	return nil
}

const productCategoryColumnsNoId = `name, description, parent_id, created_at`
const productCategoryColumns = `id, ` + productCategoryColumnsNoId

type ProductCategoryList []ProductCategory

func (m *ProductCategory) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ProductCategory row")
	}
	return row.Scan(&m.ID, &m.Name, &m.Description, &m.ParentID, &m.CreatedAt)
}

func (l *ProductCategoryList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ProductCategory rows")
	}
	*l = ProductCategoryList{}
	for rows.Next() {
		var item ProductCategory
		err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.ParentID, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (c *ProductCategoryList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name", "description"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM product_categories"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + productCategoryColumns + ` FROM product_categories` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	c.ScanRows(rows)
	return total, nil
}

func GetProductCategoryDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM product_categories` + where + ` ORDER BY name` + params.PaginationClause()
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

func (c *ProductCategory) GetByID(id string) error {
	return c.ScanRow(RDB.QueryRow(`SELECT `+productCategoryColumns+` FROM product_categories WHERE id = ?`, id))
}

func (c *ProductCategory) Create() error {
	c.ID = uuid.Must(uuid.NewV7()).String()
	c.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO product_categories (`+productCategoryColumns+`) VALUES (?,?,?,?,?)`,
		c.ID, c.Name, c.Description, c.ParentID, c.CreatedAt)
	return err
}

func (c *ProductCategory) Update(updates map[string]interface{}) error {
	cols := map[string]string{"name": "name", "description": "description", "parentId": "parent_id"}
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
		return c.GetByID(c.ID)
	}
	args = append(args, c.ID)
	_, err := DB.Exec("UPDATE product_categories SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return c.GetByID(c.ID)
}

func (c *ProductCategory) Delete() error {
	res, err := DB.Exec("DELETE FROM product_categories WHERE id = ?", c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
