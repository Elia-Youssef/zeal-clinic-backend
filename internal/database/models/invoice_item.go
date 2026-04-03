package models

import (
	"database/sql"
	"errors"
)

type InvoiceItem struct {
	ID        string  `json:"id"`
	InvoiceID string  `json:"invoiceId"`
	ItemType  string  `json:"itemType"`
	ItemID    string  `json:"itemId"`
	Quantity  int     `json:"quantity"`
	Amount    float64 `json:"amount"`
	Notes     string  `json:"notes"`
	CreatedAt Date    `json:"createdAt"`
}

const invoiceItemColumnsNoId = `invoice_id, item_type, item_id, quantity, amount, notes, created_at`
const invoiceItemColumns = `id, ` + invoiceItemColumnsNoId

type InvoiceItemList []InvoiceItem

func (m *InvoiceItem) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil InvoiceItem row")
	}
	return row.Scan(&m.ID, &m.InvoiceID, &m.ItemType, &m.ItemID, &m.Quantity, &m.Amount, &m.Notes, &m.CreatedAt)
}

func (l *InvoiceItemList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil InvoiceItem rows")
	}
	*l = InvoiceItemList{}
	for rows.Next() {
		var item InvoiceItem
		err := rows.Scan(&item.ID, &item.InvoiceID, &item.ItemType, &item.ItemID, &item.Quantity, &item.Amount, &item.Notes, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (ii *InvoiceItem) GetByInvoice(invoiceID string) ([]InvoiceItem, error) {
	rows, err := RDB.Query(`SELECT `+invoiceItemColumns+` FROM invoice_items WHERE invoice_id = ? ORDER BY created_at`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list InvoiceItemList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
}
