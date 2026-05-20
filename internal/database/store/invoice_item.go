package store

import (
	"database/sql"
	"errors"
)

type InvoiceItem struct {
	ID          string  `json:"id"`
	InvoiceID   string  `json:"invoiceId"`
	ItemType    string  `json:"itemType"`
	ItemID      string  `json:"itemId"`
	Quantity    int     `json:"quantity"`
	Amount      float64 `json:"amount"`
	FinalAmount float64 `json:"finalAmount"`
	Notes       string  `json:"notes"`
	CreatedAt   Date    `json:"createdAt"`

	// Gift-line inputs (write-only on Invoice.Create when ItemType == "gift").
	// Either GiftPatientID or GiftCode must be set; the gift's redeemable value
	// equals Amount (gross, ignoring quantity). After Create, ItemID points at
	// the resulting Discount row.
	GiftPatientID *string `json:"giftPatientId,omitempty"`
	GiftCode      *string `json:"giftCode,omitempty"`
	GiftName      string  `json:"giftName,omitempty"`

	// Transient join
	ItemName string `json:"itemName"`
}

const invoiceItemColumnsNoId = `invoice_id, item_type, item_id, quantity, amount, final_amount, notes, created_at`
const invoiceItemColumns = `id, ` + invoiceItemColumnsNoId

type InvoiceItemList []InvoiceItem

func (m *InvoiceItem) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil InvoiceItem row")
	}
	return row.Scan(&m.ID, &m.InvoiceID, &m.ItemType, &m.ItemID, &m.Quantity, &m.Amount, &m.FinalAmount, &m.Notes, &m.CreatedAt)
}

func (l *InvoiceItemList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil InvoiceItem rows")
	}
	*l = InvoiceItemList{}
	for rows.Next() {
		var item InvoiceItem
		err := rows.Scan(&item.ID, &item.InvoiceID, &item.ItemType, &item.ItemID, &item.Quantity, &item.Amount, &item.FinalAmount, &item.Notes, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (ii *InvoiceItemList) GetByInvoice(invoiceID string) error {
	rows, err := RDB.Query(`SELECT `+invoiceItemColumns+` FROM invoice_items WHERE invoice_id = ? ORDER BY created_at`, invoiceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := ii.ScanRows(rows); err != nil {
		return err
	}
	for idx, i := range *ii {
		switch i.ItemType {
		case "product":
			RDB.QueryRow(`SELECT name FROM products WHERE id = ?`, i.ItemID).Scan(&(*ii)[idx].ItemName)
		case "procedure":
			RDB.QueryRow(`SELECT name FROM procedures WHERE id = ?`, i.ItemID).Scan(&(*ii)[idx].ItemName)
		case "gift":
			RDB.QueryRow(`SELECT name FROM discounts WHERE id = ?`, i.ItemID).Scan(&(*ii)[idx].ItemName)
		default:
			(*ii)[idx].ItemName = "Other"
		}
	}
	return nil
}
