package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type StockAdjustment struct {
	ID             string `json:"id"`
	SKU            string `json:"sku"`
	Type           string `json:"type"`
	QuantityChange int    `json:"quantityChange"`
	Reason         string `json:"reason"`
	Timestamp      string `json:"timestamp"`
}

func (a *StockAdjustment) GetAll() ([]StockAdjustment, error) {
	rows, err := DB.Query("SELECT id, sku, type, quantity_change, reason, timestamp FROM stock_adjustments ORDER BY timestamp DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var adjs []StockAdjustment
	for rows.Next() {
		var adj StockAdjustment
		if err := rows.Scan(&adj.ID, &adj.SKU, &adj.Type, &adj.QuantityChange, &adj.Reason, &adj.Timestamp); err != nil {
			return nil, err
		}
		adjs = append(adjs, adj)
	}
	return adjs, rows.Err()
}

func (a *StockAdjustment) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	a.ID = uuid.New().String()
	a.Timestamp = time.Now().Format(time.RFC3339)

	_, err = tx.Exec(`INSERT INTO stock_adjustments (id, sku, type, quantity_change, reason, timestamp)
		VALUES (?,?,?,?,?,?)`, a.ID, a.SKU, a.Type, a.QuantityChange, a.Reason, a.Timestamp)
	if err != nil {
		return fmt.Errorf("insert adjustment: %w", err)
	}

	_, err = tx.Exec("UPDATE inventory_items SET quantity = quantity + ? WHERE sku = ?", a.QuantityChange, a.SKU)
	if err != nil {
		return fmt.Errorf("update inventory: %w", err)
	}

	return tx.Commit()
}
