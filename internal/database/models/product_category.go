package models

import (
	"database/sql"

	"github.com/google/uuid"
)

type ProductCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    string `json:"parentId"`
	CreatedAt   string `json:"createdAt"`
}

func (c *ProductCategory) GetAll() ([]ProductCategory, error) {
	rows, err := DB.Query(`SELECT id, name, description, parent_id, created_at FROM product_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ProductCategory
	for rows.Next() {
		var cat ProductCategory
		if err := rows.Scan(&cat.ID, &cat.Name, &cat.Description, &cat.ParentID, &cat.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, cat)
	}
	return items, rows.Err()
}

func (c *ProductCategory) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, name, description, parent_id, created_at FROM product_categories WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Description, &c.ParentID, &c.CreatedAt)
	return err
}

func (c *ProductCategory) Create() error {
	c.ID = uuid.New().String()
	_, err := DB.Exec(`INSERT INTO product_categories (id, name, description, parent_id, created_at) VALUES (?,?,?,?,?)`,
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
