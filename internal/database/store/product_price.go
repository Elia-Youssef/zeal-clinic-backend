package store

import (
	"database/sql"
	"errors"
)

type ProductPrice struct {
	ID        string  `json:"id"`
	ProductID string  `json:"productId"`
	Price     float64 `json:"price"`
	IsActive  bool    `json:"isActive"`
	CreatedAt Date    `json:"createdAt"`
}

const productPriceColumns = `id, product_id, price, is_active, created_at`

type ProductPriceList []ProductPrice

func (m *ProductPrice) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ProductPrice row")
	}
	var isActive int
	err := row.Scan(&m.ID, &m.ProductID, &m.Price, &isActive, &m.CreatedAt)
	if err != nil {
		return err
	}
	m.IsActive = isActive == 1
	return nil
}

func (l *ProductPriceList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ProductPrice rows")
	}
	*l = ProductPriceList{}
	for rows.Next() {
		var item ProductPrice
		var isActive int
		err := rows.Scan(&item.ID, &item.ProductID, &item.Price, &isActive, &item.CreatedAt)
		if err != nil {
			continue
		}
		item.IsActive = isActive == 1
		*l = append(*l, item)
	}
	return nil
}

func (l *ProductPriceList) GetByProduct(productID string, params ListParams) (int, error) {
	var total int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM product_prices WHERE product_id = ?`, productID).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"price":     "price",
		"isActive":  "is_active",
		"createdAt": "created_at",
	}, "is_active DESC, created_at DESC")
	rows, err := RDB.Query(`SELECT `+productPriceColumns+` FROM product_prices WHERE product_id = ?`+order+params.PaginationClause(), productID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if err := l.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, nil
}
