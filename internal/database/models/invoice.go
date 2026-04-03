package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (inv *Invoice) IsValid() error {
	if msg := validation.Positive(inv.Amount, "Amount"); msg != "" {
		return validation.Errors{"amount": msg}
	}
	return nil
}

type Invoice struct {
	ID            string  `json:"id"`
	InvoiceNumber int     `json:"invoiceNumber"`
	FromBalanceID string  `json:"fromBalanceId"`
	ToBalanceID   string  `json:"toBalanceId"`
	Amount        float64 `json:"amount"`
	CurrencyID    string  `json:"currencyId"`
	Notes         string  `json:"notes"`
	CreatedBy     string  `json:"createdBy"`
	CreatedAt     Date    `json:"createdAt"`
	UpdatedAt     Date    `json:"updatedAt"`
	// Nested
	Items []InvoiceItem `json:"items,omitempty"`
	// Joined fields
	FromEntityName string `json:"fromEntityName,omitempty"`
	ToEntityName   string `json:"toEntityName,omitempty"`
}

const invoiceColumnsNoId = `invoice_number, from_balance_id, to_balance_id, amount, currency_id, notes, created_by, created_at, updated_at`
const invoiceColumns = `id, ` + invoiceColumnsNoId

type InvoiceList []Invoice

func (m *Invoice) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Invoice row")
	}
	return row.Scan(&m.ID, &m.InvoiceNumber, &m.FromBalanceID, &m.ToBalanceID, &m.Amount, &m.CurrencyID,
		&m.Notes, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
}

func (l *InvoiceList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Invoice rows")
	}
	*l = InvoiceList{}
	for rows.Next() {
		var item Invoice
		err := rows.Scan(&item.ID, &item.InvoiceNumber, &item.FromBalanceID, &item.ToBalanceID, &item.Amount, &item.CurrencyID,
			&item.Notes, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (inv *Invoice) GetAll(filterBalanceID string) ([]Invoice, error) {
	query := `SELECT ` + invoiceColumns + ` FROM invoices`
	var args []interface{}
	if filterBalanceID != "" {
		query += " WHERE from_balance_id = ? OR to_balance_id = ?"
		args = append(args, filterBalanceID, filterBalanceID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}

	var list InvoiceList
	if err := list.ScanRows(rows); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range list {
		list[i].Items, _ = (&InvoiceItem{}).GetByInvoice(list[i].ID)
		if list[i].FromBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].FromBalanceID).Scan(&list[i].FromEntityName)
		}
		if list[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].ToBalanceID).Scan(&list[i].ToEntityName)
		}
	}
	return list, nil
}

func (inv *Invoice) GetByID(id string) error {
	err := inv.ScanRow(RDB.QueryRow(`SELECT `+invoiceColumns+` FROM invoices WHERE id = ?`, id))
	if err != nil {
		return err
	}
	inv.Items, _ = (&InvoiceItem{}).GetByInvoice(inv.ID)
	if inv.FromBalanceID != "" {
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, inv.FromBalanceID).Scan(&inv.FromEntityName)
	}
	if inv.ToBalanceID != "" {
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, inv.ToBalanceID).Scan(&inv.ToEntityName)
	}
	return nil
}

func (inv *Invoice) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var nextNum int
	err = tx.QueryRow(`SELECT COALESCE(MAX(invoice_number), 0) + 1 FROM invoices`).Scan(&nextNum)
	if err != nil {
		return fmt.Errorf("invoice counter: %w", err)
	}

	inv.ID = uuid.Must(uuid.NewV7()).String()
	inv.InvoiceNumber = nextNum
	now := DateNow()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	_, err = tx.Exec(`INSERT INTO invoices (`+invoiceColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.InvoiceNumber, inv.FromBalanceID, inv.ToBalanceID, inv.Amount, inv.CurrencyID,
		inv.Notes, inv.CreatedBy, inv.CreatedAt, inv.UpdatedAt)
	if err != nil {
		return err
	}

	for i := range inv.Items {
		inv.Items[i].ID = uuid.Must(uuid.NewV7()).String()
		inv.Items[i].InvoiceID = inv.ID
		inv.Items[i].CreatedAt = now
		_, err = tx.Exec(`INSERT INTO invoice_items (`+invoiceItemColumns+`) VALUES (?,?,?,?,?,?,?,?)`,
			inv.Items[i].ID, inv.Items[i].InvoiceID, inv.Items[i].ItemType, inv.Items[i].ItemID,
			inv.Items[i].Quantity, inv.Items[i].Amount, inv.Items[i].Notes, inv.Items[i].CreatedAt)
		if err != nil {
			return err
		}
	}

	// Update product quantities based on invoice direction relative to self
	bal := Balance{}
	fromEntityType := bal.GetEntityType(inv.FromBalanceID, tx)
	toEntityType := bal.GetEntityType(inv.ToBalanceID, tx)
	for _, item := range inv.Items {
		if item.ItemType != "product" || item.ItemID == "" {
			continue
		}
		var delta int
		if toEntityType == "self" {
			delta += item.Quantity
		}
		if fromEntityType == "self" {
			delta -= item.Quantity
		}
		if delta != 0 {
			if err := (&Product{ID: item.ItemID}).AdjustQuantity(delta, tx); err != nil {
				return fmt.Errorf("update product quantity: %w", err)
			}
		}
	}

	// Create balance transaction (double-entry: debit from, credit to)
	bt := BalanceTransaction{
		FromBalanceID: inv.FromBalanceID,
		ToBalanceID:   inv.ToBalanceID,
		Amount:        inv.Amount,
		CurrencyID:    inv.CurrencyID,
		Description:   fmt.Sprintf("Invoice #%d", inv.InvoiceNumber),
		CreatedBy:     inv.CreatedBy,
	}
	if err := bt.CreateWithTx(tx); err != nil {
		return fmt.Errorf("create transaction: %w", err)
	}

	return tx.Commit()
}

func (inv *Invoice) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"notes": "notes",
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
		return inv.GetByID(inv.ID)
	}
	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, inv.ID)
	_, err := DB.Exec("UPDATE invoices SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return inv.GetByID(inv.ID)
}

func (inv *Invoice) GetClientInvoices(patientID string) ([]Invoice, error) {
	query := `SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.currency_id, i.notes, i.created_by, i.created_at, i.updated_at
		FROM invoices i
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'`
	var args []interface{}
	if patientID != "" {
		query += ` AND tb.entity_id = ?`
		args = append(args, patientID)
	}
	query += ` ORDER BY i.created_at DESC`

	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list InvoiceList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Items, _ = (&InvoiceItem{}).GetByInvoice(list[i].ID)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].FromBalanceID).Scan(&list[i].FromEntityName)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].ToBalanceID).Scan(&list[i].ToEntityName)
	}
	return list, nil
}

func (inv *Invoice) GetSupplierInvoices(supplierID string) ([]Invoice, error) {
	query := `SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.currency_id, i.notes, i.created_by, i.created_at, i.updated_at
		FROM invoices i
		JOIN balances fb ON fb.id = i.from_balance_id
		WHERE fb.entity_type = 'supplier'`
	var args []interface{}
	if supplierID != "" {
		query += ` AND fb.entity_id = ?`
		args = append(args, supplierID)
	}
	query += ` ORDER BY i.created_at DESC`

	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list InvoiceList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Items, _ = (&InvoiceItem{}).GetByInvoice(list[i].ID)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].FromBalanceID).Scan(&list[i].FromEntityName)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].ToBalanceID).Scan(&list[i].ToEntityName)
	}
	return list, nil
}

func (inv *Invoice) GetByItem(itemID, itemType string) ([]Invoice, error) {
	query := `SELECT DISTINCT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.currency_id, i.notes, i.created_by, i.created_at, i.updated_at
		FROM invoices i
		JOIN invoice_items ii ON ii.invoice_id = i.id
		WHERE ii.item_id = ? AND ii.item_type = ?
		ORDER BY i.created_at DESC`

	rows, err := RDB.Query(query, itemID, itemType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list InvoiceList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Items, _ = (&InvoiceItem{}).GetByInvoice(list[i].ID)
		if list[i].FromBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].FromBalanceID).Scan(&list[i].FromEntityName)
		}
		if list[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, list[i].ToBalanceID).Scan(&list[i].ToEntityName)
		}
	}
	return list, nil
}

func (inv *Invoice) Delete() error {
	res, err := DB.Exec("DELETE FROM invoices WHERE id = ?", inv.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
