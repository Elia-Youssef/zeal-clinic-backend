package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// Product Allergy Conflict

const productAllergyConflictColumnsNoId = `product_id, allergy_id, notes, created_at`
const productAllergyConflictColumns = `id, ` + productAllergyConflictColumnsNoId

type ProductAllergyConflict struct {
	ID        string `json:"id"`
	ProductID string `json:"productId"`
	AllergyID string `json:"allergyId"`
	Notes     string `json:"notes"`
	CreatedAt Date   `json:"createdAt"`
	// Joined fields
	AllergyName string `json:"allergyName,omitempty"`
}

func (c *ProductAllergyConflict) IsValid() error {
	if msg := validation.Required(c.AllergyID, "Allergy ID"); msg != "" {
		return validation.Errors{"allergyId": msg}
	}
	return nil
}

type ProductAllergyConflictList []ProductAllergyConflict

func (m *ProductAllergyConflict) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ProductAllergyConflict row")
	}
	return row.Scan(&m.ID, &m.ProductID, &m.AllergyID, &m.Notes, &m.CreatedAt, &m.AllergyName)
}

func (l *ProductAllergyConflictList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ProductAllergyConflict rows")
	}
	*l = ProductAllergyConflictList{}
	for rows.Next() {
		var item ProductAllergyConflict
		err := rows.Scan(&item.ID, &item.ProductID, &item.AllergyID, &item.Notes, &item.CreatedAt, &item.AllergyName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (c *ProductAllergyConflictList) GetByProduct(productID string) error {
	rows, err := RDB.Query(`SELECT pac.id, pac.product_id, pac.allergy_id, pac.notes, pac.created_at, a.name
		FROM product_allergy_conflicts pac JOIN allergies a ON a.id = pac.allergy_id
		WHERE pac.product_id = ? ORDER BY a.name`, productID)
	if err != nil {
		return err
	}
	defer rows.Close()

	c.ScanRows(rows)
	return rows.Err()
}

func (c *ProductAllergyConflict) Create() error {
	c.ID = uuid.Must(uuid.NewV7()).String()
	c.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO product_allergy_conflicts (id, product_id, allergy_id, notes, created_at) VALUES (?,?,?,?,?)`,
		c.ID, c.ProductID, c.AllergyID, c.Notes, c.CreatedAt)
	return err
}

func (c *ProductAllergyConflict) Delete() error {
	res, err := DB.Exec("DELETE FROM product_allergy_conflicts WHERE id = ?", c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
