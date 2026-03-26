package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Balance struct {
	ID         string  `json:"id"`
	EntityType string  `json:"entityType"`
	EntityID   string  `json:"entityId"`
	EntityName string  `json:"entityName"`
	Currency   string  `json:"currency"`
	Amount     float64 `json:"amount"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

type BalanceTransaction struct {
	ID              string  `json:"id"`
	DebitBalanceID  string  `json:"debitBalanceId"`
	CreditBalanceID string  `json:"creditBalanceId"`
	Amount          float64 `json:"amount"`
	Currency        string  `json:"currency"`
	ExchangeRateID  string  `json:"exchangeRateId"`
	ReferenceType   string  `json:"referenceType"`
	ReferenceID     string  `json:"referenceId"`
	Description     string  `json:"description"`
	CreatedBy       string  `json:"createdBy"`
	CreatedAt       string  `json:"createdAt"`
	// Joined fields
	DebitEntityName  string `json:"debitEntityName,omitempty"`
	CreditEntityName string `json:"creditEntityName,omitempty"`
}

type Invoice struct {
	ID                        string  `json:"id"`
	InvoiceNumber             int     `json:"invoiceNumber"`
	Type                      string  `json:"type"`
	Status                    string  `json:"status"`
	PatientID                 string  `json:"patientId"`
	PatientProcedureSessionID string  `json:"patientProcedureSessionId"`
	ProductSKU                string  `json:"productSku"`
	BalanceTransactionID      string  `json:"balanceTransactionId"`
	Amount                    float64 `json:"amount"`
	Currency                  string  `json:"currency"`
	PaymentMethod             string  `json:"paymentMethod"`
	Items                     string  `json:"items"`
	DueDate                   string  `json:"dueDate"`
	PaidDate                  string  `json:"paidDate"`
	Notes                     string  `json:"notes"`
	CreatedBy                 string  `json:"createdBy"`
	CreatedAt                 string  `json:"createdAt"`
	UpdatedAt                 string  `json:"updatedAt"`
	// Joined fields
	PatientName string `json:"patientName,omitempty"`
}

// Balance

func (b *Balance) GetOrCreate() error {
	err := DB.QueryRow(`SELECT id, entity_type, entity_id, entity_name, currency, amount, created_at, updated_at
		FROM balances WHERE entity_type = ? AND entity_id = ? AND currency = ?`,
		b.EntityType, b.EntityID, b.Currency).
		Scan(&b.ID, &b.EntityType, &b.EntityID, &b.EntityName, &b.Currency, &b.Amount, &b.CreatedAt, &b.UpdatedAt)
	if err == sql.ErrNoRows {
		now := time.Now().Format(time.RFC3339)
		b.ID = uuid.New().String()
		b.Amount = 0
		b.CreatedAt = now
		b.UpdatedAt = now
		_, err = DB.Exec(`INSERT INTO balances (id, entity_type, entity_id, entity_name, currency, amount, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?)`,
			b.ID, b.EntityType, b.EntityID, b.EntityName, b.Currency, b.Amount, b.CreatedAt, b.UpdatedAt)
		return err
	}
	return err
}

func (b *Balance) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, entity_type, entity_id, entity_name, currency, amount, created_at, updated_at
		FROM balances WHERE id = ?`, id).
		Scan(&b.ID, &b.EntityType, &b.EntityID, &b.EntityName, &b.Currency, &b.Amount, &b.CreatedAt, &b.UpdatedAt)
	return err
}

func (b *Balance) GetAll(entityType string) ([]Balance, error) {
	query := `SELECT id, entity_type, entity_id, entity_name, currency, amount, created_at, updated_at FROM balances`
	var args []interface{}
	if entityType != "" {
		query += " WHERE entity_type = ?"
		args = append(args, entityType)
	}
	query += " ORDER BY entity_type, entity_name"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Balance
	for rows.Next() {
		var bal Balance
		if err := rows.Scan(&bal.ID, &bal.EntityType, &bal.EntityID, &bal.EntityName, &bal.Currency, &bal.Amount, &bal.CreatedAt, &bal.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, bal)
	}
	return items, rows.Err()
}

// Balance Transactions

func (bt *BalanceTransaction) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	bt.ID = uuid.New().String()
	bt.CreatedAt = time.Now().Format(time.RFC3339)

	_, err = tx.Exec(`INSERT INTO balance_transactions (id, debit_balance_id, credit_balance_id, amount, currency,
		exchange_rate_id, reference_type, reference_id, description, created_by, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		bt.ID, bt.DebitBalanceID, bt.CreditBalanceID, bt.Amount, bt.Currency,
		bt.ExchangeRateID, bt.ReferenceType, bt.ReferenceID, bt.Description, bt.CreatedBy, bt.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	now := time.Now().Format(time.RFC3339)
	_, err = tx.Exec(`UPDATE balances SET amount = amount - ?, updated_at = ? WHERE id = ?`, bt.Amount, now, bt.DebitBalanceID)
	if err != nil {
		return fmt.Errorf("debit balance: %w", err)
	}
	_, err = tx.Exec(`UPDATE balances SET amount = amount + ?, updated_at = ? WHERE id = ?`, bt.Amount, now, bt.CreditBalanceID)
	if err != nil {
		return fmt.Errorf("credit balance: %w", err)
	}

	return tx.Commit()
}

func (bt *BalanceTransaction) GetByBalanceID(balanceID string) ([]BalanceTransaction, error) {
	rows, err := DB.Query(`SELECT bt.id, bt.debit_balance_id, bt.credit_balance_id, bt.amount, bt.currency,
		bt.exchange_rate_id, bt.reference_type, bt.reference_id, bt.description, bt.created_by, bt.created_at,
		db.entity_name, cb.entity_name
		FROM balance_transactions bt
		JOIN balances db ON db.id = bt.debit_balance_id
		JOIN balances cb ON cb.id = bt.credit_balance_id
		WHERE bt.debit_balance_id = ? OR bt.credit_balance_id = ?
		ORDER BY bt.created_at DESC`, balanceID, balanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []BalanceTransaction
	for rows.Next() {
		var t BalanceTransaction
		if err := rows.Scan(&t.ID, &t.DebitBalanceID, &t.CreditBalanceID, &t.Amount, &t.Currency,
			&t.ExchangeRateID, &t.ReferenceType, &t.ReferenceID, &t.Description, &t.CreatedBy, &t.CreatedAt,
			&t.DebitEntityName, &t.CreditEntityName); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

func (bt *BalanceTransaction) GetAll(limit int) ([]BalanceTransaction, error) {
	query := `SELECT bt.id, bt.debit_balance_id, bt.credit_balance_id, bt.amount, bt.currency,
		bt.exchange_rate_id, bt.reference_type, bt.reference_id, bt.description, bt.created_by, bt.created_at,
		db.entity_name, cb.entity_name
		FROM balance_transactions bt
		JOIN balances db ON db.id = bt.debit_balance_id
		JOIN balances cb ON cb.id = bt.credit_balance_id
		ORDER BY bt.created_at DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []BalanceTransaction
	for rows.Next() {
		var t BalanceTransaction
		if err := rows.Scan(&t.ID, &t.DebitBalanceID, &t.CreditBalanceID, &t.Amount, &t.Currency,
			&t.ExchangeRateID, &t.ReferenceType, &t.ReferenceID, &t.Description, &t.CreatedBy, &t.CreatedAt,
			&t.DebitEntityName, &t.CreditEntityName); err != nil {
			return nil, err
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

// Invoices

func (inv *Invoice) GetAll(patientID string) ([]Invoice, error) {
	query := `SELECT id, invoice_number, type, status, patient_id,
		patient_procedure_session_id, product_sku, balance_transaction_id,
		amount, currency, payment_method, items, due_date, paid_date,
		notes, created_by, created_at, updated_at
		FROM invoices`
	var args []interface{}
	if patientID != "" {
		query += " WHERE patient_id = ?"
		args = append(args, patientID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Invoice
	for rows.Next() {
		var i Invoice
		if err := rows.Scan(&i.ID, &i.InvoiceNumber, &i.Type, &i.Status, &i.PatientID,
			&i.PatientProcedureSessionID, &i.ProductSKU, &i.BalanceTransactionID,
			&i.Amount, &i.Currency, &i.PaymentMethod, &i.Items, &i.DueDate, &i.PaidDate,
			&i.Notes, &i.CreatedBy, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	return items, rows.Err()
}

func (inv *Invoice) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, invoice_number, type, status, patient_id,
		patient_procedure_session_id, product_sku, balance_transaction_id,
		amount, currency, payment_method, items, due_date, paid_date,
		notes, created_by, created_at, updated_at
		FROM invoices WHERE id = ?`, id).
		Scan(&inv.ID, &inv.InvoiceNumber, &inv.Type, &inv.Status, &inv.PatientID,
			&inv.PatientProcedureSessionID, &inv.ProductSKU, &inv.BalanceTransactionID,
			&inv.Amount, &inv.Currency, &inv.PaymentMethod, &inv.Items, &inv.DueDate, &inv.PaidDate,
			&inv.Notes, &inv.CreatedBy, &inv.CreatedAt, &inv.UpdatedAt)
	return err
}

func (inv *Invoice) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var nextNum int
	err = tx.QueryRow(`UPDATE counters SET value = value + 1 WHERE name = 'invoice' RETURNING value`).Scan(&nextNum)
	if err != nil {
		tx.Exec(`INSERT OR IGNORE INTO counters (name, value) VALUES ('invoice', 0)`)
		err = tx.QueryRow(`UPDATE counters SET value = value + 1 WHERE name = 'invoice' RETURNING value`).Scan(&nextNum)
		if err != nil {
			return fmt.Errorf("invoice counter: %w", err)
		}
	}

	inv.ID = uuid.New().String()
	inv.InvoiceNumber = nextNum
	now := time.Now().Format(time.RFC3339)
	inv.CreatedAt = now
	inv.UpdatedAt = now

	_, err = tx.Exec(`INSERT INTO invoices (id, invoice_number, type, status, patient_id,
		patient_procedure_session_id, product_sku, balance_transaction_id,
		amount, currency, payment_method, items, due_date, paid_date,
		notes, created_by, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		inv.ID, inv.InvoiceNumber, inv.Type, inv.Status, inv.PatientID,
		inv.PatientProcedureSessionID, inv.ProductSKU, inv.BalanceTransactionID,
		inv.Amount, inv.Currency, inv.PaymentMethod, inv.Items, inv.DueDate, inv.PaidDate,
		inv.Notes, inv.CreatedBy, inv.CreatedAt, inv.UpdatedAt)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (inv *Invoice) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"status": "status", "balanceTransactionId": "balance_transaction_id",
		"paidDate": "paid_date", "notes": "notes", "paymentMethod": "payment_method",
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
	args = append(args, time.Now().Format(time.RFC3339))
	args = append(args, inv.ID)
	_, err := DB.Exec("UPDATE invoices SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return inv.GetByID(inv.ID)
}
