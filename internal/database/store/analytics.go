package store

import (
	"database/sql"
	"fmt"
	"strings"
)

type Analytics struct{}

func (a *Analytics) TotalPatients() (int, error) {
	var n int
	err := RDB.QueryRow(`SELECT COUNT(*) FROM patients`).Scan(&n)
	return n, err
}

func (a *Analytics) NewPatientsThisMonth() (int, error) {
	var n int
	err := RDB.QueryRow(`SELECT COUNT(*) FROM patients
		WHERE strftime('%Y-%m', created_at) = strftime('%Y-%m', 'now')`).Scan(&n)
	return n, err
}

type AppointmentCounts struct {
	Today     int `json:"today"`
	ThisWeek  int `json:"thisWeek"`
	ThisMonth int `json:"thisMonth"`
}

func (a *Analytics) AppointmentCounts() (AppointmentCounts, error) {
	var c AppointmentCounts
	err := RDB.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN date(start_time) = date('now') THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN date(start_time) >= date('now','weekday 0','-6 days')
			                  AND  date(start_time) <= date('now','weekday 0')       THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN strftime('%Y-%m', start_time) = strftime('%Y-%m','now') THEN 1 ELSE 0 END), 0)
		FROM appointments
		WHERE status != 'Cancelled'
	`).Scan(&c.Today, &c.ThisWeek, &c.ThisMonth)
	return c, err
}

func (a *Analytics) RevenueThisMonth() (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(bt.amount), 0)
		FROM balance_transactions bt
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE tb.entity_type = 'self'
		AND bt.voided_at = ''
		AND bt.transaction_type = 'payment'
		AND strftime('%Y-%m', bt.created_at) = strftime('%Y-%m', 'now')
	`).Scan(&v)
	return v, err
}

func (a *Analytics) ExpensesThisMonth() (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(bt.amount), 0)
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		WHERE fb.entity_type = 'self'
		AND bt.voided_at = ''
		AND bt.transaction_type = 'payment'
		AND strftime('%Y-%m', bt.created_at) = strftime('%Y-%m', 'now')
	`).Scan(&v)
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

func (a *Analytics) ProceduresCompletedThisMonth() (int, error) {
	var n int
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(ii.quantity), 0)
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		WHERE ii.item_type = 'procedure'
		AND strftime('%Y-%m', i.created_at) = strftime('%Y-%m', 'now')
	`).Scan(&n)
	return n, err
}

func (a *Analytics) LowStockCount() (int, error) {
	var n int
	err := RDB.QueryRow(`
		SELECT COUNT(*) FROM products WHERE quantity <= min_threshold
	`).Scan(&n)
	return n, err
}

type AppointmentCancellationRate struct {
	Total     int     `json:"total"`
	Cancelled int     `json:"cancelled"`
	Rate      float64 `json:"rate"`
}

func (a *Analytics) CancellationRateThisMonth() (AppointmentCancellationRate, error) {
	var r AppointmentCancellationRate
	err := RDB.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN status = 'Cancelled' THEN 1 ELSE 0 END), 0)
		FROM appointments
		WHERE strftime('%Y-%m', start_time) = strftime('%Y-%m', 'now')
	`).Scan(&r.Total, &r.Cancelled)
	if err != nil {
		return r, err
	}
	if r.Total > 0 {
		r.Rate = float64(r.Cancelled) / float64(r.Total)
	}
	return r, nil
}

func (a *Analytics) RecentAppointmentsToday(limit int) (AppointmentList, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := RDB.Query(appointmentSelectQuery+`
		WHERE DATE(a.start_time) = DATE('now')
		AND a.status != 'Cancelled'
		ORDER BY a.start_time DESC LIMIT ?`, limit)
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
	ProcedureID string `json:"procedureId"`
	Name        string `json:"name"`
	Count       int    `json:"count"`
}

func (a *Analytics) TopProceduresThisMonth(limit int) ([]TopProcedure, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := RDB.Query(`
		SELECT ii.item_id, pr.name, COALESCE(SUM(ii.quantity), 0) AS cnt
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN procedures pr ON pr.id = ii.item_id
		WHERE ii.item_type = 'procedure'
		AND strftime('%Y-%m', i.created_at) = strftime('%Y-%m', 'now')
		GROUP BY ii.item_id, pr.name
		ORDER BY cnt DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TopProcedure{}
	for rows.Next() {
		var t TopProcedure
		if err := rows.Scan(&t.ProcedureID, &t.Name, &t.Count); err != nil {
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
			AND date(bt.created_at) BETWEEN date(?) AND date(?)
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "bt.created_at"))
		args = []any{p.From, p.To}
	case "appointments":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COUNT(*)
			FROM appointments
			WHERE status != 'Cancelled'
			AND date(start_time) BETWEEN date(?) AND date(?)
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "start_time"))
		args = []any{p.From, p.To}
	case "new-patients":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COUNT(*)
			FROM patients
			WHERE date(created_at) BETWEEN date(?) AND date(?)
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "created_at"))
		args = []any{p.From, p.To}
	case "procedures-completed":
		query = fmt.Sprintf(`
			SELECT %s AS bucket, COALESCE(SUM(ii.quantity), 0)
			FROM invoice_items ii
			JOIN invoices i ON i.id = ii.invoice_id
			WHERE ii.item_type = 'procedure'
			AND date(i.created_at) BETWEEN date(?) AND date(?)
			GROUP BY bucket ORDER BY bucket`, fmt.Sprintf(bucketExpr, "i.created_at"))
		args = []any{p.From, p.To}
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
