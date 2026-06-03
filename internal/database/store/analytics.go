package store

import (
	"database/sql"
	"fmt"
	"strings"
)

type Analytics struct{}

func (a *Analytics) NewPatients(from, to string) (int, error) {
	var n int
	err := RDB.QueryRow(`SELECT COUNT(*) FROM patients
		WHERE created_at >= ? AND created_at < ?`, RangeStart(from), RangeEnd(to)).Scan(&n)
	return n, err
}

func (a *Analytics) AppointmentCount(from, to string) (int, error) {
	var n int
	err := RDB.QueryRow(`
		SELECT COUNT(*)
		FROM appointments
		WHERE status NOT IN ('Cancelled','Rescheduled')
		AND start_time >= ? AND start_time < ?
	`, RangeStart(from), RangeEnd(to)).Scan(&n)
	return n, err
}

func (a *Analytics) Revenue(from, to string) (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(bt.amount), 0)
		FROM balance_transactions bt
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE tb.entity_type = 'self'
		AND bt.voided_at = ''
		AND bt.transaction_type = 'payment'
		AND bt.created_at >= ? AND bt.created_at < ?
	`, RangeStart(from), RangeEnd(to)).Scan(&v)
	return v, err
}

func (a *Analytics) Expenses(from, to string) (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(bt.amount), 0)
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		WHERE fb.entity_type = 'self'
		AND bt.voided_at = ''
		AND bt.transaction_type = 'payment'
		AND bt.created_at >= ? AND bt.created_at < ?
	`, RangeStart(from), RangeEnd(to)).Scan(&v)
	return v, err
}

func (a *Analytics) OutstandingReceivables() (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0) FROM balances
		WHERE entity_type = 'patient' AND amount > 0
	`).Scan(&v)
	return v, err
}

func (a *Analytics) LowStockCount() (int, error) {
	var n int
	err := RDB.QueryRow(`
		SELECT COUNT(*) FROM products WHERE quantity <= min_threshold
	`).Scan(&n)
	return n, err
}

func (a *Analytics) RecentAppointmentsToday(limit int) (AppointmentList, error) {
	if limit <= 0 {
		limit = 5
	}
	dayStart, dayEnd := ClinicTodayBounds()
	rows, err := RDB.Query(appointmentSelectQuery+`
		WHERE a.start_time >= ? AND a.start_time < ?
		AND a.status NOT IN ('Cancelled','Rescheduled')
		ORDER BY a.start_time DESC LIMIT ?`, dayStart, dayEnd, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := AppointmentList{}
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	if err := list.LoadProcedures(); err != nil {
		return nil, err
	}
	return list, nil
}

func (a *Analytics) RecentTransactions(limit int) (BalanceTransactionList, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := RDB.Query(`SELECT bt.id, bt.from_balance_id, bt.to_balance_id, bt.amount, bt.currency_id,
		bt.transaction_type, bt.transaction_method, bt.source_type, bt.source_id,
		bt.description, bt.created_by, bt.created_at, bt.voided_at,
		fb.entity_name, tb.entity_name
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE tb.entity_type = 'self'
		AND bt.voided_at = ''
		ORDER BY bt.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := BalanceTransactionList{}
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, nil
}

type TopProcedure struct {
	ProcedureID string  `json:"procedureId"`
	Name        string  `json:"name"`
	Count       int     `json:"count"`
	Amount      float64 `json:"amount"`
}

// TopProcedures ranks billed procedures over the range. by="revenue" orders by
// total final_amount; any other value orders by quantity. Both figures are
// always returned so the frontend can re-sort without a second query.
func (a *Analytics) TopProcedures(from, to string, limit int, by string) ([]TopProcedure, error) {
	if limit <= 0 {
		limit = 5
	}
	order := "cnt DESC"
	if by == "revenue" {
		order = "amt DESC"
	}
	rows, err := RDB.Query(`
		SELECT ii.item_id, pr.name,
			COALESCE(SUM(ii.quantity), 0) AS cnt,
			COALESCE(SUM(ii.final_amount), 0) AS amt
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN procedures pr ON pr.id = ii.item_id
		WHERE ii.item_type = 'procedure'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''
		GROUP BY ii.item_id, pr.name
		ORDER BY `+order+` LIMIT ?`, RangeStart(from), RangeEnd(to), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TopProcedure{}
	for rows.Next() {
		var t TopProcedure
		if err := rows.Scan(&t.ProcedureID, &t.Name, &t.Count, &t.Amount); err != nil {
			continue
		}
		items = append(items, t)
	}
	return items, rows.Err()
}

type SeriesPoint struct {
	Bucket string  `json:"bucket"`
	Value  float64 `json:"value"`
}

type SeriesParams struct {
	Metric  string
	From    string
	To      string
	GroupBy string
}

func (a *Analytics) Series(p SeriesParams) ([]SeriesPoint, error) {
	bucketExpr, err := bucketExpression(p.GroupBy)
	if err != nil {
		return nil, err
	}

	var query string
	var args []any
	switch p.Metric {
	case "revenue":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COALESCE(SUM(bt.amount), 0)
			FROM balance_transactions bt
			JOIN balances tb ON tb.id = bt.to_balance_id
			WHERE tb.entity_type = 'self'
			AND bt.voided_at = ''
			AND bt.transaction_type = 'payment'
			AND bt.created_at >= ? AND bt.created_at < ?
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "bt.created_at"))
		args = []any{RangeStart(p.From), RangeEnd(p.To)}
	case "expenses":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COALESCE(SUM(bt.amount), 0)
			FROM balance_transactions bt
			JOIN balances fb ON fb.id = bt.from_balance_id
			WHERE fb.entity_type = 'self'
			AND bt.voided_at = ''
			AND bt.transaction_type = 'payment'
			AND bt.created_at >= ? AND bt.created_at < ?
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "bt.created_at"))
		args = []any{RangeStart(p.From), RangeEnd(p.To)}
	case "appointments":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COUNT(*)
			FROM appointments
			WHERE status NOT IN ('Cancelled','Rescheduled')
			AND start_time >= ? AND start_time < ?
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "start_time"))
		args = []any{RangeStart(p.From), RangeEnd(p.To)}
	case "new-patients":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COUNT(*)
			FROM patients
			WHERE created_at >= ? AND created_at < ?
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "created_at"))
		args = []any{RangeStart(p.From), RangeEnd(p.To)}
	case "procedures-completed":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COALESCE(SUM(ii.quantity), 0)
			FROM invoice_items ii
			JOIN invoices i ON i.id = ii.invoice_id
			WHERE ii.item_type = 'procedure'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "i.created_at"))
		args = []any{RangeStart(p.From), RangeEnd(p.To)}
	default:
		return nil, fmt.Errorf("unsupported metric %q", p.Metric)
	}

	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []SeriesPoint{}
	for rows.Next() {
		var pt SeriesPoint
		var v sql.NullFloat64
		if err := rows.Scan(&pt.Bucket, &v); err != nil {
			continue
		}
		pt.Value = v.Float64
		points = append(points, pt)
	}
	return points, rows.Err()
}

func bucketExpression(groupBy string) (string, error) {
	switch strings.ToLower(groupBy) {
	case "", "day":
		return "strftime('%%Y-%%m-%%d', %s)", nil
	case "week":
		return "strftime('%%Y-W%%W', %s)", nil
	case "month":
		return "strftime('%%Y-%%m', %s)", nil
	default:
		return "", fmt.Errorf("unsupported groupBy %q", groupBy)
	}
}
