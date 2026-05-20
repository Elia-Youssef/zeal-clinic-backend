package store

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
	// DiscountID points to an "offer" Discount applied invoice-wide.
	// DiscountValue is the resolved monetary discount (computed from the
	// discount's value_type/value at create time, capped at Amount).
	DiscountID    string  `json:"discountId,omitempty"`
	DiscountValue float64 `json:"discountValue"`
	FinalAmount   float64 `json:"finalAmount"`
	CurrencyID    string  `json:"currencyId"`
	Notes         string  `json:"notes"`
	CreatedBy     string  `json:"createdBy"`
	CreatedAt     Date    `json:"createdAt"`
	UpdatedAt     Date    `json:"updatedAt"`
	// Nested
	Items InvoiceItemList `json:"items,omitempty"`
	// Joined fields
	FromEntityID   string `json:"fromEntityId,omitempty"`
	ToEntityID     string `json:"toEntityId,omitempty"`
	FromEntityName string `json:"fromEntityName,omitempty"`
	ToEntityName   string `json:"toEntityName,omitempty"`
}

const invoiceColumnsNoId = `invoice_number, from_balance_id, to_balance_id, amount, discount_id, discount_value, final_amount, currency_id, notes, created_by, created_at, updated_at`
const invoiceColumns = `id, ` + invoiceColumnsNoId

type InvoiceList []Invoice

func (m *Invoice) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Invoice row")
	}
	return row.Scan(&m.ID, &m.InvoiceNumber, &m.FromBalanceID, &m.ToBalanceID, &m.Amount,
		&m.DiscountID, &m.DiscountValue, &m.FinalAmount, &m.CurrencyID,
		&m.Notes, &m.CreatedBy, &m.CreatedAt, &m.UpdatedAt)
}

func (l *InvoiceList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Invoice rows")
	}
	*l = InvoiceList{}
	for rows.Next() {
		var item Invoice
		err := rows.Scan(&item.ID, &item.InvoiceNumber, &item.FromBalanceID, &item.ToBalanceID, &item.Amount,
			&item.DiscountID, &item.DiscountValue, &item.FinalAmount, &item.CurrencyID,
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
	var args []any
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
			RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityID, &(*inv)[i].FromEntityName)
		}
		if (*inv)[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityID, &(*inv)[i].ToEntityName)
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
		RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, inv.FromBalanceID).Scan(&inv.FromEntityID, &inv.FromEntityName)
	}
	if inv.ToBalanceID != "" {
		RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, inv.ToBalanceID).Scan(&inv.ToEntityID, &inv.ToEntityName)
	}
	return nil
}

func (inv *Invoice) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	bal := Balance{}
	fromEntityType := bal.GetEntityType(inv.FromBalanceID, tx)
	toEntityType := bal.GetEntityType(inv.ToBalanceID, tx)

	// Scope the invoice counter to the non-self party (patient vs supplier),
	// so client and supplier invoices maintain independent sequences.
	partyEntityType := fromEntityType
	if partyEntityType == "self" {
		partyEntityType = toEntityType
	}

	// If the caller provided a specific invoice_number, honor it (after
	// checking uniqueness within the same party scope). Otherwise pick the
	// next sequential number. A caller-supplied number that's higher than
	// the current max effectively bumps the sequence: future auto-assigned
	// numbers will continue from there.
	if inv.InvoiceNumber > 0 {
		var dup int
		if err := tx.QueryRow(`SELECT COUNT(*)
			FROM invoices i
			JOIN balances fb ON fb.id = i.from_balance_id
			JOIN balances tb ON tb.id = i.to_balance_id
			WHERE i.invoice_number = ?
			AND (fb.entity_type = ? OR tb.entity_type = ?)`,
			inv.InvoiceNumber, partyEntityType, partyEntityType).Scan(&dup); err != nil {
			return fmt.Errorf("check invoice number: %w", err)
		}
		if dup > 0 {
			return fmt.Errorf("%w: invoice number %d already exists", ErrConflict, inv.InvoiceNumber)
		}
	} else {
		if err := tx.QueryRow(`SELECT COALESCE(MAX(i.invoice_number), 0) + 1
			FROM invoices i
			JOIN balances fb ON fb.id = i.from_balance_id
			JOIN balances tb ON tb.id = i.to_balance_id
			WHERE fb.entity_type = ? OR tb.entity_type = ?`, partyEntityType, partyEntityType).Scan(&inv.InvoiceNumber); err != nil {
			return fmt.Errorf("invoice counter: %w", err)
		}
	}

	inv.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	inv.CreatedAt = now
	inv.UpdatedAt = now

	// 1) Materialize each line. Gift lines create a Discount row up front so
	//    the line's item_id can point at it. Sum line amounts into Amount.
	var total float64
	for i := range inv.Items {
		item := &inv.Items[i]
		item.ID = uuid.Must(uuid.NewV7()).String()
		item.InvoiceID = inv.ID
		item.CreatedAt = now
		item.FinalAmount = item.Amount

		if item.ItemType == "gift" {
			gift, err := createGiftFromLine(tx, item, now)
			if err != nil {
				return err
			}
			item.ItemID = gift.ID
		}
		total += item.Amount
	}
	inv.Amount = total

	// 2) Resolve invoice-level offer discount (if any). Caps at Amount.
	if inv.DiscountID != "" {
		var dType, vType string
		var value float64
		if err := tx.QueryRow(`SELECT discount_type, value_type, value FROM discounts WHERE id = ?`, inv.DiscountID).Scan(&dType, &vType, &value); err != nil {
			return fmt.Errorf("discount %s: %w", inv.DiscountID, err)
		}
		if dType != "offer" {
			return fmt.Errorf("invoice discount must be of type 'offer', got %q", dType)
		}
		var dv float64
		if vType == "percentage" {
			dv = inv.Amount * value / 100
		} else {
			dv = value
		}
		if dv > inv.Amount {
			dv = inv.Amount
		}
		inv.DiscountValue = dv
	}
	inv.FinalAmount = inv.Amount - inv.DiscountValue

	// 3) Insert invoice + items.
	if _, err := tx.Exec(`INSERT INTO invoices (`+invoiceColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.InvoiceNumber, inv.FromBalanceID, inv.ToBalanceID, inv.Amount,
		inv.DiscountID, inv.DiscountValue, inv.FinalAmount, inv.CurrencyID,
		inv.Notes, inv.CreatedBy, inv.CreatedAt, inv.UpdatedAt); err != nil {
		return err
	}
	for i := range inv.Items {
		item := &inv.Items[i]
		if _, err := tx.Exec(`INSERT INTO invoice_items (`+invoiceItemColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
			item.ID, item.InvoiceID, item.ItemType, item.ItemID,
			item.Quantity, item.Amount, item.FinalAmount, item.Notes, item.CreatedAt); err != nil {
			return err
		}
	}

	// 4) Adjust product stock based on direction relative to self.
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

	// 5) Charge transaction for what's actually owed (final, post-discount).
	bt := BalanceTransaction{
		FromBalanceID:   inv.FromBalanceID,
		ToBalanceID:     inv.ToBalanceID,
		Amount:          inv.FinalAmount,
		CurrencyID:      inv.CurrencyID,
		TransactionType: "charge",
		SourceType:      "invoice",
		SourceID:        inv.ID,
		Description:     fmt.Sprintf("Invoice #%d", inv.InvoiceNumber),
		CreatedBy:       inv.CreatedBy,
	}
	if inv.DiscountID != "" {
		bt.TransactionMethod = "discount"
	}
	if err := bt.CreateWithTx(tx); err != nil {
		return fmt.Errorf("create transaction: %w", err)
	}

	// 6) Auto-apply each gift that was assigned to a specific patient at
	//    creation. Direction = patient to self with type=adjustment so the
	//    recipient's balance moves toward credit (negative) and total_in/out
	//    is untouched (no real cash flowed).
	for _, item := range inv.Items {
		if item.ItemType != "gift" || item.GiftPatientID == nil || *item.GiftPatientID == "" {
			continue
		}
		if err := applyGiftToPatientWithTx(tx, *item.GiftPatientID, inv.CurrencyID, item.Amount,
			item.ItemID, "invoice", inv.ID,
			fmt.Sprintf("Gift card credit (Invoice #%d)", inv.InvoiceNumber), inv.CreatedBy); err != nil {
			return fmt.Errorf("apply gift: %w", err)
		}
	}

	return tx.Commit()
}

// createGiftFromLine creates a gift Discount row from an invoice line. The
// line's Amount is the gift's redeemable value. Notes copy into description.
func createGiftFromLine(tx *sql.Tx, item *InvoiceItem, now Date) (*Discount, error) {
	hasPatient := item.GiftPatientID != nil && *item.GiftPatientID != ""
	hasCode := item.GiftCode != nil && *item.GiftCode != ""
	if hasPatient == hasCode {
		return nil, errors.New("gift line must have exactly one of giftPatientId or giftCode")
	}
	name := "Gift Card"
	if item.GiftName != "" {
		name = item.GiftName
	} else if item.Notes != "" {
		name = item.Notes
	}
	gift := &Discount{
		Name:         name,
		Description:  item.Notes,
		DiscountType: "gift",
		ValueType:    "fixed",
		Value:        item.Amount,
		PatientID:    item.GiftPatientID,
		Code:         item.GiftCode,
		IsActive:     1,
	}
	if err := gift.CreateWithTx(tx); err != nil {
		return nil, fmt.Errorf("create gift discount: %w", err)
	}
	return gift, nil
}

// applyGiftToPatientWithTx records the credit application for a gift on a
// patient's balance and marks the gift redeemed. sourceType/sourceID link the
// transaction to whatever triggered the apply (the originating invoice when
// the gift had a patient_id at creation, or the discount itself when redeemed
// later via code).
func applyGiftToPatientWithTx(tx *sql.Tx, patientID, currencyID string, amount float64,
	giftID, sourceType, sourceID, description, createdBy string) error {
	patientBalance, err := resolvePatientBalanceWithTx(tx, patientID, currencyID)
	if err != nil {
		return err
	}
	selfBalanceID, err := selfBalanceIDForCurrency(tx, currencyID)
	if err != nil {
		return err
	}
	bt := BalanceTransaction{
		FromBalanceID:     patientBalance.ID,
		ToBalanceID:       selfBalanceID,
		Amount:            amount,
		CurrencyID:        currencyID,
		TransactionType:   "adjustment",
		TransactionMethod: "discount",
		SourceType:        sourceType,
		SourceID:          sourceID,
		Description:       description,
		CreatedBy:         createdBy,
	}
	if err := bt.CreateWithTx(tx); err != nil {
		return err
	}
	gift := Discount{ID: giftID}
	return gift.MarkRedeemedWithTx(tx)
}

// ApplyGiftByCode redeems a gift by its code, crediting the given patient's
// balance. Returns the updated discount.
func ApplyGiftByCode(code, patientID, currencyID, createdBy string) (*Discount, error) {
	if code == "" || patientID == "" || currencyID == "" {
		return nil, errors.New("code, patientId and currencyId are required")
	}
	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var gift Discount
	row := tx.QueryRow(`SELECT `+discountColumns+` FROM discounts WHERE code = ?`, code)
	if err := gift.ScanRow(row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if gift.DiscountType != "gift" {
		return nil, errors.New("not a gift discount")
	}
	if gift.RedeemedAt != nil {
		return nil, errors.New("gift already redeemed")
	}
	if gift.IsActive == 0 {
		return nil, errors.New("gift is inactive")
	}

	if err := applyGiftToPatientWithTx(tx, patientID, currencyID, gift.Value,
		gift.ID, "discount", gift.ID,
		fmt.Sprintf("Redeemed gift card %q", gift.Name), createdBy); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`UPDATE discounts SET patient_id = ?, updated_at = ? WHERE id = ?`,
		patientID, DateNow(), gift.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	pid := patientID
	gift.PatientID = &pid
	return &gift, nil
}

func resolvePatientBalanceWithTx(tx *sql.Tx, patientID, currencyID string) (*Balance, error) {
	var firstName, lastName string
	if err := tx.QueryRow(`SELECT first_name, last_name FROM patients WHERE id = ?`, patientID).Scan(&firstName, &lastName); err != nil {
		return nil, fmt.Errorf("patient %s: %w", patientID, err)
	}
	now := DateNow()
	id := uuid.Must(uuid.NewV7()).String()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO balances (`+balanceColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, "patient", patientID, firstName+" "+lastName, currencyID, 0, 0, 0, now, now); err != nil {
		return nil, err
	}
	var b Balance
	if err := b.ScanRow(tx.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type = 'patient' AND entity_id = ? AND currency_id = ?`, patientID, currencyID)); err != nil {
		return nil, err
	}
	return &b, nil
}

func selfBalanceIDForCurrency(tx *sql.Tx, currencyID string) (string, error) {
	var id string
	if err := tx.QueryRow(`SELECT id FROM balances WHERE entity_type = 'self' AND currency_id = ? LIMIT 1`, currencyID).Scan(&id); err != nil {
		return "", fmt.Errorf("self balance for currency %s: %w", currencyID, err)
	}
	return id, nil
}

// UpdateItemAmount sets a single invoice item's amount, recomputes the
// invoice total (re-applying any invoice-level discount the same way Create
// does), and replaces the charge balance transaction with one reflecting the
// new final amount. Other transactions sourced from this invoice (gift
// auto-applies, supplier payments) are untouched.
func (inv *Invoice) UpdateItemAmount(itemID string, amount float64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var (
		invoiceNumber              int
		discountID, currencyID     string
		fromBalanceID, toBalanceID string
		createdBy                  string
	)
	err = tx.QueryRow(`SELECT invoice_number, discount_id, currency_id,
		from_balance_id, to_balance_id, created_by
		FROM invoices WHERE id = ?`, inv.ID).
		Scan(&invoiceNumber, &discountID, &currencyID,
			&fromBalanceID, &toBalanceID, &createdBy)
	if err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	res, err := tx.Exec(`UPDATE invoice_items
		SET amount = ?, final_amount = ?
		WHERE id = ? AND invoice_id = ?`, amount, amount, itemID, inv.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	var newAmount float64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM invoice_items WHERE invoice_id = ?`, inv.ID).Scan(&newAmount); err != nil {
		return err
	}

	var newDiscountValue float64
	if discountID != "" {
		var dType, vType string
		var value float64
		if err := tx.QueryRow(`SELECT discount_type, value_type, value FROM discounts WHERE id = ?`, discountID).Scan(&dType, &vType, &value); err != nil {
			return fmt.Errorf("discount %s: %w", discountID, err)
		}
		if vType == "percentage" {
			newDiscountValue = newAmount * value / 100
		} else {
			newDiscountValue = value
		}
		if newDiscountValue > newAmount {
			newDiscountValue = newAmount
		}
	}
	newFinalAmount := newAmount - newDiscountValue

	now := DateNow()
	if _, err := tx.Exec(`UPDATE invoices SET amount = ?, discount_value = ?, final_amount = ?, updated_at = ? WHERE id = ?`,
		newAmount, newDiscountValue, newFinalAmount, now, inv.ID); err != nil {
		return err
	}

	// Void + recreate the charge transaction. Gift auto-apply rows are
	// transaction_type='adjustment' so they're excluded by the filter.
	var oldCharge BalanceTransaction
	err = tx.QueryRow(`SELECT `+balanceTransactionColumns+` FROM balance_transactions
		WHERE source_type = 'invoice' AND source_id = ? AND transaction_type = 'charge'
		AND voided_at = ''
		LIMIT 1`, inv.ID).
		Scan(&oldCharge.ID, &oldCharge.FromBalanceID, &oldCharge.ToBalanceID, &oldCharge.Amount, &oldCharge.CurrencyID,
			&oldCharge.TransactionType, &oldCharge.TransactionMethod, &oldCharge.SourceType, &oldCharge.SourceID,
			&oldCharge.Description, &oldCharge.CreatedBy, &oldCharge.CreatedAt, &oldCharge.VoidedAt)
	if err != nil {
		return fmt.Errorf("load charge transaction: %w", err)
	}
	if err := oldCharge.voidAndRecalculateWithTx(tx); err != nil {
		return fmt.Errorf("void charge transaction: %w", err)
	}

	newCharge := BalanceTransaction{
		FromBalanceID:   fromBalanceID,
		ToBalanceID:     toBalanceID,
		Amount:          newFinalAmount,
		CurrencyID:      currencyID,
		TransactionType: "charge",
		SourceType:      "invoice",
		SourceID:        inv.ID,
		Description:     fmt.Sprintf("Invoice #%d", invoiceNumber),
		CreatedBy:       createdBy,
	}
	if discountID != "" {
		newCharge.TransactionMethod = "discount"
	}
	if err := newCharge.CreateWithTx(tx); err != nil {
		return fmt.Errorf("create charge transaction: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return inv.GetByID(inv.ID)
}

func (inv *Invoice) Update(updates map[string]any) error {
	cols := map[string]string{
		"notes": "notes",
	}
	setClauses := ""
	var args []any
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
		i.amount, i.discount_id, i.discount_value, i.final_amount, i.currency_id,
		i.notes, i.created_by, i.created_at, i.updated_at
		FROM invoices i
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'`
	var args []any
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
		RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityID, &(*inv)[i].FromEntityName)
		RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityID, &(*inv)[i].ToEntityName)
	}
	return nil
}

func (inv *InvoiceList) GetSupplierInvoices(supplierID string) error {
	query := `SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.discount_id, i.discount_value, i.final_amount, i.currency_id,
		i.notes, i.created_by, i.created_at, i.updated_at
		FROM invoices i
		JOIN balances fb ON fb.id = i.from_balance_id
		WHERE fb.entity_type = 'supplier'`
	var args []any
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
		RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityID, &(*inv)[i].FromEntityName)
		RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityID, &(*inv)[i].ToEntityName)
	}
	return nil
}

func (inv *InvoiceList) GetAllByType(entityType string, params ListParams) (int, error) {
	baseFrom := ` FROM invoices i
		JOIN balances fb ON fb.id = i.from_balance_id
		JOIN balances tb ON tb.id = i.to_balance_id`
	where := " WHERE 1=1"
	var args []any
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

	order := params.OrderClause(map[string]string{
		"invoiceNumber":  "i.invoice_number",
		"amount":         "i.amount",
		"discountValue":  "i.discount_value",
		"finalAmount":    "i.final_amount",
		"currencyId":     "i.currency_id",
		"createdAt":      "i.created_at",
		"updatedAt":      "i.updated_at",
		"fromEntityName": "fb.entity_name",
		"toEntityName":   "tb.entity_name",
	}, "i.created_at DESC")
	query := `SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.discount_id, i.discount_value, i.final_amount, i.currency_id,
		i.notes, i.created_by, i.created_at, i.updated_at` +
		baseFrom + where + order + params.PaginationClause()
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
			RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityID, &(*inv)[i].FromEntityName)
		}
		if (*inv)[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityID, &(*inv)[i].ToEntityName)
		}
	}
	return total, nil
}

func (inv *InvoiceList) GetByItem(itemID, itemType string) error {
	query := `SELECT DISTINCT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id,
		i.amount, i.discount_id, i.discount_value, i.final_amount, i.currency_id,
		i.notes, i.created_by, i.created_at, i.updated_at
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
			RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].FromBalanceID).Scan(&(*inv)[i].FromEntityID, &(*inv)[i].FromEntityName)
		}
		if (*inv)[i].ToBalanceID != "" {
			RDB.QueryRow(`SELECT COALESCE(entity_id, ''), entity_name FROM balances WHERE id = ?`, (*inv)[i].ToBalanceID).Scan(&(*inv)[i].ToEntityID, &(*inv)[i].ToEntityName)
		}
	}
	return nil
}

// Delete reverses every side effect of Invoice.Create (product stock, the
// charge transaction, any auto-applied gift transactions), then removes the
// invoice and any gift discounts it created. Refuses if a gift was already
// redeemed (via code) outside this invoice's flow, since the redemption tx
// would be left dangling.
func (inv *Invoice) Delete() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var fromBalance, toBalance string
	err = tx.QueryRow(`SELECT from_balance_id, to_balance_id FROM invoices WHERE id = ?`, inv.ID).
		Scan(&fromBalance, &toBalance)
	if err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	bal := Balance{}
	fromEntityType := bal.GetEntityType(fromBalance, tx)
	toEntityType := bal.GetEntityType(toBalance, tx)

	// Walk items to reverse stock and identify gift discounts to delete.
	rows, err := tx.Query(`SELECT id, item_type, item_id, quantity FROM invoice_items WHERE invoice_id = ?`, inv.ID)
	if err != nil {
		return err
	}
	type itemRow struct {
		id, itemType, itemID string
		quantity             int
	}
	var items []itemRow
	for rows.Next() {
		var it itemRow
		if err := rows.Scan(&it.id, &it.itemType, &it.itemID, &it.quantity); err != nil {
			rows.Close()
			return err
		}
		items = append(items, it)
	}
	rows.Close()

	var giftIDs []string
	for _, it := range items {
		if it.itemType == "product" && it.itemID != "" {
			var delta int
			if toEntityType == "self" {
				delta -= it.quantity
			}
			if fromEntityType == "self" {
				delta += it.quantity
			}
			if delta != 0 {
				if _, err := tx.Exec(`UPDATE products SET quantity = quantity + ? WHERE id = ?`, delta, it.itemID); err != nil {
					return fmt.Errorf("reverse product quantity: %w", err)
				}
			}
		}
		if it.itemType == "gift" && it.itemID != "" {
			giftIDs = append(giftIDs, it.itemID)
		}
	}

	// Refuse delete if any gift created here has a redemption tx that wasn't
	// also sourced from this invoice (i.e. a code-based redemption).
	for _, gid := range giftIDs {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM balance_transactions
			WHERE source_type = 'discount' AND source_id = ? AND voided_at = ''`, gid).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("cannot delete invoice: gift card has been redeemed")
		}
	}

	// Void every balance transaction sourced from this invoice (the charge
	// plus any auto-applied gifts).
	chargeRows, err := tx.Query(`SELECT id, from_balance_id, to_balance_id, amount, transaction_type FROM balance_transactions
		WHERE source_type = 'invoice' AND source_id = ? AND voided_at = ''`, inv.ID)
	if err != nil {
		return err
	}
	var charges []BalanceTransaction
	for chargeRows.Next() {
		var ch BalanceTransaction
		if err := chargeRows.Scan(&ch.ID, &ch.FromBalanceID, &ch.ToBalanceID, &ch.Amount, &ch.TransactionType); err != nil {
			chargeRows.Close()
			return err
		}
		charges = append(charges, ch)
	}
	chargeRows.Close()
	for i := range charges {
		if err := charges[i].voidAndRecalculateWithTx(tx); err != nil {
			return fmt.Errorf("void charge transaction: %w", err)
		}
	}

	// Drop gift discount rows. Items cascade with the invoice.
	for _, gid := range giftIDs {
		if _, err := tx.Exec(`DELETE FROM discounts WHERE id = ?`, gid); err != nil {
			return fmt.Errorf("delete gift discount: %w", err)
		}
	}

	res, err := tx.Exec(`DELETE FROM invoices WHERE id = ?`, inv.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}

	return tx.Commit()
}
