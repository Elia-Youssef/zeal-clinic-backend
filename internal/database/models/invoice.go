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
	FinalAmount   float64 `json:"finalAmount"`
	CurrencyID    string  `json:"currencyId"`
	Notes         string  `json:"notes"`
	CreatedBy     string  `json:"createdBy"`
	CreatedAt     Date    `json:"createdAt"`
	UpdatedAt     Date    `json:"updatedAt"`
	// Nested
	Items InvoiceItemList `json:"items,omitempty"`
	// Joined fields
	FromEntityName string `json:"fromEntityName,omitempty"`
	ToEntityName   string `json:"toEntityName,omitempty"`
}

const invoiceColumnsNoId = `invoice_number, from_balance_id, to_balance_id, amount, final_amount, currency_id, notes, created_by, created_at, updated_at`
const invoiceColumns = `id, ` + invoiceColumnsNoId

type InvoiceList []Invoice

func (m *Invoice) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Invoice row")
	}
	return row.Scan(&m.ID, &m.InvoiceNumber, &m.FromBalanceID, &m.ToBalanceID, &m.Amount, &m.FinalAmount, &m.CurrencyID,
		&m.Notes, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
}

func (l *InvoiceList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Invoice rows")
	}
	*l = InvoiceList{}
	for rows.Next() {
		var item Invoice
		err := rows.Scan(&item.ID, &item.InvoiceNumber, &item.FromBalanceID, &item.ToBalanceID, &item.Amount, &item.FinalAmount, &item.CurrencyID,
			&item.Notes, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (inv *InvoiceList) GetAll(filterBalanceID string) error {
	query := `SELECT ` + invoiceColumns + ` FROM invoices`
	var args []interface{}
	if filterBalanceID != "" {
		query += " WHERE from_balance_id = ? OR to_balance_id = ?"
		args = append(args, filterBalanceID, filterBalanceID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := RDB.Query(query, args...)
	if err != nil {
		return err
	}

	if err := inv.ScanRows(rows); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range *inv {
		(*inv)[i].Items.GetByInvoice((*inv)[i].ID)
		if (*inv)[i].FromBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityName)
		}
		if (*inv)[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityName)
		}
	}
	return nil
}

func (inv *Invoice) GetByID(id string) error {
	err := inv.ScanRow(RDB.QueryRow(`SELECT `+invoiceColumns+` FROM invoices WHERE id = ?`, id))
	if err != nil {
		return err
	}
	inv.Items.GetByInvoice(inv.ID)
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
	if err := tx.QueryRow(`SELECT COALESCE(MAX(invoice_number), 0) + 1 FROM invoices`).Scan(&nextNum); err != nil {
		return fmt.Errorf("invoice counter: %w", err)
	}

	inv.ID = uuid.Must(uuid.NewV7()).String()
	inv.InvoiceNumber = nextNum
	now := DateNow()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	// Resolve per-item discount values; compute line final amounts and invoice totals.
	var total, finalTotal float64
	hasDiscount := false
	for i := range inv.Items {
		item := &inv.Items[i]
		item.FinalAmount = item.Amount
		if item.DiscountID != "" {
			var valueType string
			var value float64
			if err := tx.QueryRow(`SELECT value_type, value FROM discounts WHERE id = ?`, item.DiscountID).Scan(&valueType, &value); err != nil {
				return fmt.Errorf("discount %s: %w", item.DiscountID, err)
			}
			var dv float64
			if valueType == "percentage" {
				dv = item.Amount * value / 100
			} else {
				dv = value
			}
			if dv > item.Amount {
				dv = item.Amount
			}
			item.DiscountValue = dv
			item.FinalAmount = item.Amount - dv
			hasDiscount = true
		}
		total += item.Amount
		finalTotal += item.FinalAmount
	}
	inv.Amount = total
	inv.FinalAmount = finalTotal

	if _, err := tx.Exec(`INSERT INTO invoices (`+invoiceColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.InvoiceNumber, inv.FromBalanceID, inv.ToBalanceID, inv.Amount, inv.FinalAmount, inv.CurrencyID,
		inv.Notes, inv.CreatedBy, inv.CreatedAt, inv.UpdatedAt); err != nil {
		return err
	}

	for i := range inv.Items {
		item := &inv.Items[i]
		item.ID = uuid.Must(uuid.NewV7()).String()
		item.InvoiceID = inv.ID
		item.CreatedAt = now

		if _, err := tx.Exec(`INSERT INTO invoice_items (`+invoiceItemColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
			item.ID, item.InvoiceID, item.ItemType, item.ItemID,
			item.Quantity, item.Amount, item.FinalAmount, item.Notes, item.CreatedAt); err != nil {
			return err
		}

		if item.DiscountID != "" {
			if _, err := tx.Exec(`INSERT INTO invoice_item_discounts (`+invoiceItemDiscountColumns+`) VALUES (?,?,?,?,?)`,
				uuid.Must(uuid.NewV7()).String(), item.ID, item.DiscountID, item.DiscountValue, now); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE discounts SET current_usages = current_usages + 1, updated_at = ? WHERE id = ?`, now, item.DiscountID); err != nil {
				return err
			}
		}
	}

	// Adjust product stock based on direction relative to self.
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

	// Record the double-entry charge for what's actually owed (discounted total).
	bt := BalanceTransaction{
		FromBalanceID:   inv.FromBalanceID,
		ToBalanceID:     inv.ToBalanceID,
		Amount:          inv.FinalAmount,
		CurrencyID:      inv.CurrencyID,
		TransactionType: "charge",
		Description:     fmt.Sprintf("Invoice #%d", inv.InvoiceNumber),
		CreatedBy:       inv.CreatedBy,
	}
	if hasDiscount {
		bt.TransactionMethod = "discount"
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

func (inv *InvoiceList) GetClientInvoices(patientID string) error {
	query := `SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.final_amount, i.currency_id, i.notes, i.created_by, i.created_at, i.updated_at
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
		return err
	}
	defer rows.Close()

	if err := inv.ScanRows(rows); err != nil {
		return err
	}
	for i := range *inv {
		(*inv)[i].Items.GetByInvoice((*inv)[i].ID)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityName)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityName)
	}
	return nil
}

func (inv *InvoiceList) GetSupplierInvoices(supplierID string) error {
	query := `SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.final_amount, i.currency_id, i.notes, i.created_by, i.created_at, i.updated_at
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
		return err
	}
	defer rows.Close()

	if err := inv.ScanRows(rows); err != nil {
		return err
	}
	for i := range *inv {
		(*inv)[i].Items.GetByInvoice((*inv)[i].ID)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityName)
		RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityName)
	}
	return nil
}

func (inv *InvoiceList) GetAllByType(entityType string, params ListParams) (int, error) {
	baseFrom := ` FROM invoices i
		JOIN balances fb ON fb.id = i.from_balance_id
		JOIN balances tb ON tb.id = i.to_balance_id`
	where := " WHERE 1=1"
	var args []interface{}
	if entityType != "" {
		where += " AND (fb.entity_type = ? OR tb.entity_type = ?)"
		args = append(args, entityType, entityType)
	}
	if fc, fa := params.FilterClause("fb.entity_name", "tb.entity_name", "i.notes"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*)"+baseFrom+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.final_amount, i.currency_id, i.notes, i.created_by, i.created_at, i.updated_at` +
		baseFrom + where + ` ORDER BY i.created_at DESC` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := inv.ScanRows(rows); err != nil {
		return 0, err
	}
	for i := range *inv {
		(*inv)[i].Items.GetByInvoice((*inv)[i].ID)
		if (*inv)[i].FromBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityName)
		}
		if (*inv)[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityName)
		}
	}
	return total, nil
}

func (inv *InvoiceList) GetByItem(itemID, itemType string) error {
	query := `SELECT DISTINCT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.final_amount, i.currency_id, i.notes, i.created_by, i.created_at, i.updated_at
		FROM invoices i
		JOIN invoice_items ii ON ii.invoice_id = i.id
		WHERE ii.item_id = ? AND ii.item_type = ?
		ORDER BY i.created_at DESC`

	rows, err := RDB.Query(query, itemID, itemType)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := inv.ScanRows(rows); err != nil {
		return err
	}
	for i := range *inv {
		(*inv)[i].Items.GetByInvoice((*inv)[i].ID)
		if (*inv)[i].FromBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityName)
		}
		if (*inv)[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityName)
		}
	}
	return nil
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
