package models

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type InvoiceItemDiscount struct {
	ID            string  `json:"id"`
	InvoiceItemID string  `json:"invoiceItemId"`
	DiscountID    string  `json:"discountId"`
	DiscountValue float64 `json:"discountValue"`
	CreatedAt     Date    `json:"createdAt"`
	// Transient
	DiscountName string `json:"discountName,omitempty"`
}

const invoiceItemDiscountColumnsNoId = `invoice_item_id, discount_id, discount_value, created_at`
const invoiceItemDiscountColumns = `id, ` + invoiceItemDiscountColumnsNoId

type InvoiceItemDiscountList []InvoiceItemDiscount

func (l *InvoiceItemDiscountList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil InvoiceItemDiscount rows")
	}
	*l = InvoiceItemDiscountList{}
	for rows.Next() {
		var item InvoiceItemDiscount
		if err := rows.Scan(&item.ID, &item.InvoiceItemID, &item.DiscountID, &item.DiscountValue, &item.CreatedAt); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (l *InvoiceItemDiscountList) GetByInvoiceItem(invoiceItemID string) error {
	rows, err := RDB.Query(`SELECT `+invoiceItemDiscountColumns+` FROM invoice_item_discounts WHERE invoice_item_id = ? ORDER BY created_at`, invoiceItemID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := l.ScanRows(rows); err != nil {
		return err
	}
	for i := range *l {
		RDB.QueryRow(`SELECT name FROM discounts WHERE id = ?`, (*l)[i].DiscountID).Scan(&(*l)[i].DiscountName)
	}
	return nil
}

func (iid *InvoiceItemDiscount) Create() error {
	iid.ID = uuid.Must(uuid.NewV7()).String()
	iid.CreatedAt = DateNow()

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO invoice_item_discounts (`+invoiceItemDiscountColumns+`) VALUES (?,?,?,?,?)`,
		iid.ID, iid.InvoiceItemID, iid.DiscountID, iid.DiscountValue, iid.CreatedAt)
	if err != nil {
		return err
	}

	// Increment discount usage
	_, err = tx.Exec(`UPDATE discounts SET current_usages = current_usages + 1, updated_at = ? WHERE id = ?`, DateNow(), iid.DiscountID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func DeleteInvoiceItemDiscount(id string) error {
	res, err := DB.Exec("DELETE FROM invoice_item_discounts WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
