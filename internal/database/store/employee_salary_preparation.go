package store

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// One employee's prepared salary for one period. A "run" is just the set of
// rows sharing the same (period_start, period_end); there is no run table.
type EmployeeSalaryPreparation struct {
	ID             string  `json:"id"`
	EmployeeID     string  `json:"employeeId"`
	EmployeeName   string  `json:"employeeName,omitempty"`
	PeriodStart    Date    `json:"periodStart"`
	PeriodEnd      Date    `json:"periodEnd"`
	SalaryID       string  `json:"salaryId"`
	TransactionID  string  `json:"transactionId"`
	CurrencyID     string  `json:"currencyId"`
	BaseSalary     float64 `json:"baseSalary"`
	Adjustment     float64 `json:"adjustment"`
	PreparedAmount float64 `json:"preparedAmount"`
	Notes          string  `json:"notes"`
	CreatedBy      string  `json:"createdBy"`
	CreatedAt      Date    `json:"createdAt"`
}

const employeeSalaryPreparationColumns = `id, employee_id, period_start, period_end, salary_id, transaction_id, currency_id, base_salary, adjustment, prepared_amount, notes, created_by, created_at`

// PrepareEmployeeSalaries snapshots each employee's latest active salary for
// the period. Idempotent per employee: re-runs skip employees that already
// have a row and insert only the missing ones.
func PrepareEmployeeSalaries(periodStart Date, periodEnd Date, notes string, createdBy string) ([]EmployeeSalaryPreparation, error) {
	if _, _, err := parseDateRange(periodStart, periodEnd); err != nil {
		return nil, err
	}

	tx, err := DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, first_name || ' ' || last_name FROM employees ORDER BY first_name, last_name`)
	if err != nil {
		return nil, err
	}
	type employeeRow struct{ id, name string }
	var employees []employeeRow
	for rows.Next() {
		var e employeeRow
		if err := rows.Scan(&e.id, &e.name); err != nil {
			rows.Close()
			return nil, err
		}
		employees = append(employees, e)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	out := []EmployeeSalaryPreparation{}
	for _, e := range employees {
		var existing string
		err := tx.QueryRow(`SELECT id FROM employee_salary_preparations
			WHERE employee_id = ? AND period_start = ? AND period_end = ? LIMIT 1`,
			e.id, periodStart, periodEnd).Scan(&existing)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return nil, err
		}

		var salary EmployeeSalary
		err = tx.QueryRow(`SELECT `+employeeSalaryColumns+` FROM employee_salaries
			WHERE employee_id = ? AND effective_date <= ?
			ORDER BY effective_date DESC, created_at DESC LIMIT 1`, e.id, periodEnd).
			Scan(&salary.ID, &salary.EmployeeID, &salary.Amount, &salary.CurrencyID, new(int),
				&salary.EffectiveDate, &salary.Notes, &salary.CreatedAt, &salary.UpdatedAt)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return nil, err
		}
		salary.CurrencyID = USDCurrencyID
		if salary.Amount <= 0 {
			continue
		}

		selfBalance, err := getOrCreateBalanceWithTx(tx, "self", "self", "Clinic", salary.CurrencyID)
		if err != nil {
			return nil, err
		}
		employeeBalance, err := getOrCreateBalanceWithTx(tx, "employee", e.id, e.name, salary.CurrencyID)
		if err != nil {
			return nil, err
		}

		prep := EmployeeSalaryPreparation{
			ID:             uuid.Must(uuid.NewV7()).String(),
			EmployeeID:     e.id,
			EmployeeName:   e.name,
			PeriodStart:    periodStart,
			PeriodEnd:      periodEnd,
			SalaryID:       salary.ID,
			CurrencyID:     salary.CurrencyID,
			BaseSalary:     salary.Amount,
			PreparedAmount: salary.Amount,
			Notes:          notes,
			CreatedBy:      createdBy,
			CreatedAt:      DateNow(),
		}

		bt := BalanceTransaction{
			FromBalanceID:     employeeBalance.ID,
			ToBalanceID:       selfBalance.ID,
			Amount:            prep.PreparedAmount,
			CurrencyID:        prep.CurrencyID,
			TransactionType:   "charge",
			TransactionMethod: "other",
			SourceType:        "employee_salary_preparation",
			SourceID:          prep.ID,
			Description:       fmt.Sprintf("Salary preparation %s to %s", periodStart, periodEnd),
			CreatedBy:         createdBy,
		}
		if err := bt.CreateWithTx(tx); err != nil {
			return nil, err
		}
		prep.TransactionID = bt.ID

		if _, err := tx.Exec(`INSERT INTO employee_salary_preparations
			(`+employeeSalaryPreparationColumns+`)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			prep.ID, prep.EmployeeID, prep.PeriodStart, prep.PeriodEnd, prep.SalaryID, prep.TransactionID,
			prep.CurrencyID, prep.BaseSalary, prep.Adjustment, prep.PreparedAmount, prep.Notes, prep.CreatedBy, prep.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, prep)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func PreparedSalariesForEmployee(employeeID string, params ListParams) ([]EmployeeSalaryPreparation, int, error) {
	var total int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM employee_salary_preparations WHERE employee_id = ?`, employeeID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := RDB.Query(`SELECT `+employeeSalaryPreparationColumns+` FROM employee_salary_preparations
		WHERE employee_id = ?
		ORDER BY period_start DESC, created_at DESC`+params.PaginationClause(), employeeID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []EmployeeSalaryPreparation{}
	for rows.Next() {
		var p EmployeeSalaryPreparation
		if err := rows.Scan(&p.ID, &p.EmployeeID, &p.PeriodStart, &p.PeriodEnd, &p.SalaryID, &p.TransactionID,
			&p.CurrencyID, &p.BaseSalary, &p.Adjustment, &p.PreparedAmount, &p.Notes, &p.CreatedBy, &p.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

// SetAdjustment sets the signed adjustment, recomputes prepared_amount =
// base_salary + adjustment, and patches the linked balance transaction by the
// delta. Adjustment may be negative, but prepared_amount must stay non-negative.
func (p *EmployeeSalaryPreparation) SetAdjustment(adjustment float64) error {
	adjustment = Round2(adjustment)

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var current EmployeeSalaryPreparation
	err = tx.QueryRow(`SELECT `+employeeSalaryPreparationColumns+` FROM employee_salary_preparations WHERE id = ?`, p.ID).
		Scan(&current.ID, &current.EmployeeID, &current.PeriodStart, &current.PeriodEnd, &current.SalaryID, &current.TransactionID,
			&current.CurrencyID, &current.BaseSalary, &current.Adjustment, &current.PreparedAmount, &current.Notes, &current.CreatedBy, &current.CreatedAt)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	newAmount := Round2(current.BaseSalary + adjustment)
	if newAmount < 0 {
		return fmt.Errorf("adjustment would make prepared amount negative")
	}
	delta := Round2(newAmount - current.PreparedAmount)

	if delta != 0 && current.TransactionID != "" {
		var fromID, toID string
		if err := tx.QueryRow(`SELECT from_balance_id, to_balance_id FROM balance_transactions WHERE id = ? AND voided_at = ''`, current.TransactionID).
			Scan(&fromID, &toID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE balance_transactions SET amount = ? WHERE id = ? AND voided_at = ''`, newAmount, current.TransactionID); err != nil {
			return err
		}
		if err := RecalculateBalancesWithTx(tx, fromID, toID); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`UPDATE employee_salary_preparations SET adjustment = ?, prepared_amount = ? WHERE id = ?`,
		adjustment, newAmount, p.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	current.Adjustment = adjustment
	current.PreparedAmount = newAmount
	*p = current
	return nil
}

// Delete reverses the linked balance transaction (if any) and removes the row.
// A missing transaction is treated as already reversed.
func (p *EmployeeSalaryPreparation) Delete() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var transactionID string
	err = tx.QueryRow(`SELECT transaction_id FROM employee_salary_preparations WHERE id = ?`, p.ID).Scan(&transactionID)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if transactionID != "" {
		var bt BalanceTransaction
		err := tx.QueryRow(`SELECT from_balance_id, to_balance_id FROM balance_transactions WHERE id = ? AND voided_at = ''`, transactionID).
			Scan(&bt.FromBalanceID, &bt.ToBalanceID)
		if err == nil {
			bt.ID = transactionID
			if err := bt.voidAndRecalculateWithTx(tx); err != nil {
				return err
			}
		} else if err != sql.ErrNoRows {
			return err
		}
	}

	res, err := tx.Exec(`DELETE FROM employee_salary_preparations WHERE id = ?`, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func getOrCreateBalanceWithTx(tx *sql.Tx, entityType, entityID, entityName, currencyID string) (*Balance, error) {
	if currencyID == "" {
		currencyID = USDCurrencyID
	}
	now := DateNow()
	id := uuid.Must(uuid.NewV7()).String()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO balances (`+balanceColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		id, entityType, entityID, entityName, currencyID, 0, 0, 0, now, now); err != nil {
		return nil, err
	}
	var b Balance
	if err := tx.QueryRow(`SELECT `+balanceColumns+` FROM balances WHERE entity_type = ? AND entity_id = ? AND currency_id = ?`,
		entityType, entityID, currencyID).Scan(&b.ID, &b.EntityType, &b.EntityID, &b.EntityName, &b.CurrencyID, &b.Amount, &b.TotalIn, &b.TotalOut, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}
