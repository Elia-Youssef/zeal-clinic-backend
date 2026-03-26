package models

import "time"

type InventoryItem struct {
	SKU           string  `json:"sku"`
	Name          string  `json:"name"`
	CategoryID    string  `json:"categoryId"`
	Quantity      int     `json:"quantity"`
	MinThreshold  int     `json:"minThreshold"`
	UnitPrice     float64 `json:"unitPrice"`
	Category      string  `json:"category"`
	LastRestocked string  `json:"lastRestocked"`
	CreatedAt     string  `json:"createdAt"`
}

func (i *InventoryItem) GetAll() ([]InventoryItem, error) {
	rows, err := DB.Query("SELECT sku, name, category_id, quantity, min_threshold, unit_price, category, last_restocked, created_at FROM inventory_items ORDER BY sku")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []InventoryItem
	for rows.Next() {
		var item InventoryItem
		if err := rows.Scan(&item.SKU, &item.Name, &item.CategoryID, &item.Quantity, &item.MinThreshold, &item.UnitPrice, &item.Category, &item.LastRestocked, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (i *InventoryItem) GetBySKU(sku string) error {
	err := DB.QueryRow("SELECT sku, name, category_id, quantity, min_threshold, unit_price, category, last_restocked, created_at FROM inventory_items WHERE sku = ?", sku).
		Scan(&i.SKU, &i.Name, &i.CategoryID, &i.Quantity, &i.MinThreshold, &i.UnitPrice, &i.Category, &i.LastRestocked, &i.CreatedAt)
	return err
}

func (i *InventoryItem) Create() error {
	i.LastRestocked = time.Now().Format(time.RFC3339)
	i.CreatedAt = time.Now().Format(time.RFC3339)
	_, err := DB.Exec(`INSERT INTO inventory_items (sku, name, category_id, quantity, min_threshold, unit_price, category, last_restocked, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, i.SKU, i.Name, i.CategoryID, i.Quantity, i.MinThreshold, i.UnitPrice, i.Category, i.LastRestocked, i.CreatedAt)
	return err
}

func (i *InventoryItem) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "categoryId": "category_id", "quantity": "quantity",
		"minThreshold": "min_threshold", "unitPrice": "unit_price", "category": "category",
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
		return i.GetBySKU(i.SKU)
	}

	args = append(args, i.SKU)
	_, err := DB.Exec("UPDATE inventory_items SET "+setClauses+" WHERE sku = ?", args...)
	if err != nil {
		return err
	}
	return i.GetBySKU(i.SKU)
}
