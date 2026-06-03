package store

// Dashboard analytics: section-grouped KPI aggregates (with period-over-period
// deltas) plus the supporting list/breakdown queries. The frontend dashboard
// and the analytics PDF both read from these so the two can't drift.

// Metric is a single KPI value alongside the previous equal-length period, so
// the UI can render a delta. Change is the fractional difference
// ((value-previous)/previous); it is 0 when previous is 0.
type Metric struct {
	Value    float64 `json:"value"`
	Previous float64 `json:"previous"`
	Change   float64 `json:"change"`
}

func newMetric(cur, prev float64) Metric {
	m := Metric{Value: cur, Previous: prev}
	if prev != 0 {
		m.Change = (cur - prev) / prev
	}
	return m
}

// delta runs a single-value range query for the current and previous periods
// and packages the pair as a Metric. When the previous period can't be derived
// (pf empty), the comparison falls back to 0.
func (a *Analytics) delta(fn func(string, string) (float64, error), from, to, pf, pt string) (Metric, error) {
	cur, err := fn(from, to)
	if err != nil {
		return Metric{}, err
	}
	if pf == "" {
		return newMetric(cur, 0), nil
	}
	prev, err := fn(pf, pt)
	if err != nil {
		return Metric{}, err
	}
	return newMetric(cur, prev), nil
}

// Money

type RevenueMix struct {
	Procedures float64 `json:"procedures"`
	Products   float64 `json:"products"`
	Gifts      float64 `json:"gifts"`
	Other      float64 `json:"other"`
}

type MethodAmount struct {
	Method string  `json:"method"`
	Amount float64 `json:"amount"`
}

type MoneyAnalytics struct {
	Revenue           Metric         `json:"revenue"`
	Expenses          Metric         `json:"expenses"`
	NetProfit         Metric         `json:"netProfit"`
	AvgInvoice        Metric         `json:"avgInvoice"`
	Discounts         Metric         `json:"discounts"`
	Refunds           Metric         `json:"refunds"`
	WriteOffs         Metric         `json:"writeOffs"`
	Receivables       float64        `json:"receivables"`
	Payables          float64        `json:"payables"`
	GiftCardLiability float64        `json:"giftCardLiability"`
	RevenueMix        RevenueMix     `json:"revenueMix"`
	PaymentMix        []MethodAmount `json:"paymentMix"`
}

func (a *Analytics) Money(from, to string) (MoneyAnalytics, error) {
	pf, pt := PreviousPeriod(from, to)
	var m MoneyAnalytics
	var err error
	if m.Revenue, err = a.delta(a.Revenue, from, to, pf, pt); err != nil {
		return m, err
	}
	if m.Expenses, err = a.delta(a.Expenses, from, to, pf, pt); err != nil {
		return m, err
	}
	m.NetProfit = newMetric(m.Revenue.Value-m.Expenses.Value, m.Revenue.Previous-m.Expenses.Previous)
	if m.AvgInvoice, err = a.delta(a.avgInvoice, from, to, pf, pt); err != nil {
		return m, err
	}
	if m.Discounts, err = a.delta(a.discountsGiven, from, to, pf, pt); err != nil {
		return m, err
	}
	if m.Refunds, err = a.delta(a.refundsTotal, from, to, pf, pt); err != nil {
		return m, err
	}
	if m.WriteOffs, err = a.delta(a.writeOffsTotal, from, to, pf, pt); err != nil {
		return m, err
	}
	if m.Receivables, err = a.OutstandingReceivables(); err != nil {
		return m, err
	}
	if m.Payables, err = a.payables(); err != nil {
		return m, err
	}
	if m.GiftCardLiability, err = a.giftCardLiability(); err != nil {
		return m, err
	}
	if m.RevenueMix, err = a.revenueMix(from, to); err != nil {
		return m, err
	}
	if m.PaymentMix, err = a.paymentMix(from, to); err != nil {
		return m, err
	}
	return m, nil
}

func (a *Analytics) avgInvoice(from, to string) (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(AVG(i.final_amount), 0)
		FROM invoices i
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''
	`, RangeStart(from), RangeEnd(to)).Scan(&v)
	return v, err
}

func (a *Analytics) discountsGiven(from, to string) (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(i.discount_value), 0)
		FROM invoices i
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''
	`, RangeStart(from), RangeEnd(to)).Scan(&v)
	return v, err
}

func (a *Analytics) refundsTotal(from, to string) (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(bt.amount), 0)
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		WHERE fb.entity_type = 'self'
		AND bt.transaction_type = 'refund'
		AND bt.voided_at = ''
		AND bt.created_at >= ? AND bt.created_at < ?
	`, RangeStart(from), RangeEnd(to)).Scan(&v)
	return v, err
}

func (a *Analytics) writeOffsTotal(from, to string) (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM balance_transactions
		WHERE transaction_type = 'write-off'
		AND voided_at = ''
		AND created_at >= ? AND created_at < ?
	`, RangeStart(from), RangeEnd(to)).Scan(&v)
	return v, err
}

// payables is the total the clinic owes suppliers. Supplier invoices debit the
// supplier balance (from-side), so a negative supplier balance means we owe;
// this returns that as a positive figure.
func (a *Analytics) payables() (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(-SUM(amount), 0) FROM balances
		WHERE entity_type = 'supplier' AND amount < 0
	`).Scan(&v)
	return v, err
}

func (a *Analytics) giftCardLiability() (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(value), 0) FROM discounts
		WHERE discount_type = 'gift' AND value_type = 'fixed'
		AND redeemed_at IS NULL AND is_active = 1
	`).Scan(&v)
	return v, err
}

func (a *Analytics) revenueMix(from, to string) (RevenueMix, error) {
	var mix RevenueMix
	rows, err := RDB.Query(`
		SELECT ii.item_type, COALESCE(SUM(ii.final_amount), 0)
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''
		GROUP BY ii.item_type
	`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return mix, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var amt float64
		if err := rows.Scan(&kind, &amt); err != nil {
			continue
		}
		switch kind {
		case "procedure":
			mix.Procedures = amt
		case "product":
			mix.Products = amt
		case "gift":
			mix.Gifts = amt
		default:
			mix.Other += amt
		}
	}
	return mix, rows.Err()
}

func (a *Analytics) paymentMix(from, to string) ([]MethodAmount, error) {
	rows, err := RDB.Query(`
		SELECT bt.transaction_method, COALESCE(SUM(bt.amount), 0)
		FROM balance_transactions bt
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE tb.entity_type = 'self'
		AND bt.transaction_type = 'payment'
		AND bt.voided_at = ''
		AND bt.created_at >= ? AND bt.created_at < ?
		GROUP BY bt.transaction_method
		ORDER BY 2 DESC
	`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MethodAmount{}
	for rows.Next() {
		var ma MethodAmount
		if err := rows.Scan(&ma.Method, &ma.Amount); err != nil {
			continue
		}
		out = append(out, ma)
	}
	return out, rows.Err()
}

// Patients

type PatientsAnalytics struct {
	NewPatients       Metric  `json:"newPatients"`
	ReturningPatients Metric  `json:"returningPatients"`
	RepeatRate        Metric  `json:"repeatRate"`
	ActivePatients    float64 `json:"activePatients"`
}

func (a *Analytics) Patients(from, to string) (PatientsAnalytics, error) {
	pf, pt := PreviousPeriod(from, to)
	var p PatientsAnalytics
	var err error
	if p.NewPatients, err = a.delta(a.newPatientsF, from, to, pf, pt); err != nil {
		return p, err
	}
	if p.ReturningPatients, err = a.delta(a.returningPatients, from, to, pf, pt); err != nil {
		return p, err
	}
	if p.RepeatRate, err = a.delta(a.repeatRate, from, to, pf, pt); err != nil {
		return p, err
	}
	if p.ActivePatients, err = a.activePatients(); err != nil {
		return p, err
	}
	return p, nil
}

func (a *Analytics) newPatientsF(from, to string) (float64, error) {
	n, err := a.NewPatients(from, to)
	return float64(n), err
}

// returningPatients counts distinct patients with an appointment in the range
// who were created before the range started (i.e. not first-timers).
func (a *Analytics) returningPatients(from, to string) (float64, error) {
	var n float64
	err := RDB.QueryRow(`
		SELECT COUNT(DISTINCT a.patient_id)
		FROM appointments a
		JOIN patients p ON p.id = a.patient_id
		WHERE a.start_time >= ? AND a.start_time < ?
		AND p.created_at < ?
	`, RangeStart(from), RangeEnd(to), RangeStart(from)).Scan(&n)
	return n, err
}

// repeatRate is the share of patients seen in the range who are returning.
func (a *Analytics) repeatRate(from, to string) (float64, error) {
	var total float64
	if err := RDB.QueryRow(`
		SELECT COUNT(DISTINCT patient_id) FROM appointments
		WHERE start_time >= ? AND start_time < ?
	`, RangeStart(from), RangeEnd(to)).Scan(&total); err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	ret, err := a.returningPatients(from, to)
	if err != nil {
		return 0, err
	}
	return ret / total, nil
}

// activePatients counts distinct patients with an appointment in the last 180
// clinic-local days, a truer "active base" than the cumulative total.
func (a *Analytics) activePatients() (float64, error) {
	cutoff := DateFrom(ClinicNow().AddDate(0, 0, -180))
	var n float64
	err := RDB.QueryRow(`
		SELECT COUNT(DISTINCT patient_id) FROM appointments
		WHERE start_time >= ?
	`, cutoff).Scan(&n)
	return n, err
}

type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type Demographics struct {
	Gender    []LabelCount `json:"gender"`
	AgeBands  []LabelCount `json:"ageBands"`
	TopCities []LabelCount `json:"topCities"`
}

func (a *Analytics) Demographics(cityLimit int) (Demographics, error) {
	if cityLimit <= 0 {
		cityLimit = 5
	}
	var d Demographics
	var err error
	if d.Gender, err = a.labelCounts(`SELECT gender, COUNT(*) FROM patients GROUP BY gender ORDER BY 2 DESC`); err != nil {
		return d, err
	}
	bands, err := a.labelCounts(`
		SELECT band, COUNT(*) FROM (
			SELECT CASE
				WHEN age < 18 THEN '0-17'
				WHEN age < 30 THEN '18-29'
				WHEN age < 45 THEN '30-44'
				WHEN age < 60 THEN '45-59'
				ELSE '60+' END AS band
			FROM (
				SELECT CAST((julianday('now') - julianday(date_of_birth)) / 365.25 AS INT) AS age
				FROM patients
				WHERE date_of_birth != '' AND julianday(date_of_birth) IS NOT NULL
			)
		) GROUP BY band`)
	if err != nil {
		return d, err
	}
	d.AgeBands = orderAgeBands(bands)
	if d.TopCities, err = a.labelCounts(`
		SELECT lc.name, COUNT(*) AS c
		FROM patients p
		JOIN lebanon_cities lc ON lc.id = p.city_id
		WHERE p.city_id != ''
		GROUP BY lc.id, lc.name
		ORDER BY c DESC LIMIT ?`, cityLimit); err != nil {
		return d, err
	}
	return d, nil
}

func orderAgeBands(in []LabelCount) []LabelCount {
	order := []string{"0-17", "18-29", "30-44", "45-59", "60+"}
	found := map[string]int{}
	for _, lc := range in {
		found[lc.Label] = lc.Count
	}
	out := make([]LabelCount, 0, len(order))
	for _, band := range order {
		out = append(out, LabelCount{Label: band, Count: found[band]})
	}
	return out
}

func (a *Analytics) labelCounts(query string, args ...any) ([]LabelCount, error) {
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LabelCount{}
	for rows.Next() {
		var lc LabelCount
		if err := rows.Scan(&lc.Label, &lc.Count); err != nil {
			continue
		}
		out = append(out, lc)
	}
	return out, rows.Err()
}

type ReferralSource struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// ReferralSources groups patients acquired in the range by their referral
// source (free-text); blanks bucket into "Unknown".
func (a *Analytics) ReferralSources(from, to string) ([]ReferralSource, error) {
	rows, err := RDB.Query(`
		SELECT CASE WHEN referral_source = '' THEN 'Unknown' ELSE referral_source END AS src,
			COUNT(*) AS c
		FROM patients
		WHERE created_at >= ? AND created_at < ?
		GROUP BY src ORDER BY c DESC
	`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReferralSource{}
	for rows.Next() {
		var r ReferralSource
		if err := rows.Scan(&r.Source, &r.Count); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Operations

type AppointmentStatusCounts struct {
	Scheduled   int `json:"scheduled"`
	InProgress  int `json:"inProgress"`
	Completed   int `json:"completed"`
	Cancelled   int `json:"cancelled"`
	Rescheduled int `json:"rescheduled"`
	Total       int `json:"total"`
}

type OperationsAnalytics struct {
	Appointments        Metric                  `json:"appointments"`
	ProceduresPerformed Metric                  `json:"proceduresPerformed"`
	CancellationRate    Metric                  `json:"cancellationRate"`
	RescheduleRate      Metric                  `json:"rescheduleRate"`
	Upcoming7Days       float64                 `json:"upcoming7Days"`
	StatusBreakdown     AppointmentStatusCounts `json:"statusBreakdown"`
}

func (a *Analytics) Operations(from, to string) (OperationsAnalytics, error) {
	pf, pt := PreviousPeriod(from, to)
	var o OperationsAnalytics
	var err error
	if o.Appointments, err = a.delta(a.appointmentCountF, from, to, pf, pt); err != nil {
		return o, err
	}
	if o.ProceduresPerformed, err = a.delta(a.proceduresPerformed, from, to, pf, pt); err != nil {
		return o, err
	}
	if o.CancellationRate, err = a.delta(a.cancellationRate, from, to, pf, pt); err != nil {
		return o, err
	}
	if o.RescheduleRate, err = a.delta(a.rescheduleRate, from, to, pf, pt); err != nil {
		return o, err
	}
	if o.Upcoming7Days, err = a.upcomingCount(7); err != nil {
		return o, err
	}
	if o.StatusBreakdown, err = a.AppointmentStatusCounts(from, to); err != nil {
		return o, err
	}
	return o, nil
}

func (a *Analytics) appointmentCountF(from, to string) (float64, error) {
	n, err := a.AppointmentCount(from, to)
	return float64(n), err
}

// proceduresPerformed counts procedures on completed appointments in the range,
// sourced from appointment_procedures (actually performed), not invoices.
func (a *Analytics) proceduresPerformed(from, to string) (float64, error) {
	var n float64
	err := RDB.QueryRow(`
		SELECT COUNT(*)
		FROM appointment_procedures ap
		JOIN appointments a ON a.id = ap.appointment_id
		WHERE a.status = 'Completed'
		AND a.start_time >= ? AND a.start_time < ?
	`, RangeStart(from), RangeEnd(to)).Scan(&n)
	return n, err
}

func (a *Analytics) AppointmentStatusCounts(from, to string) (AppointmentStatusCounts, error) {
	var c AppointmentStatusCounts
	rows, err := RDB.Query(`
		SELECT status, COUNT(*) FROM appointments
		WHERE start_time >= ? AND start_time < ?
		GROUP BY status
	`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return c, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			continue
		}
		switch status {
		case "Scheduled":
			c.Scheduled = n
		case "In-Progress":
			c.InProgress = n
		case "Completed":
			c.Completed = n
		case "Cancelled":
			c.Cancelled = n
		case "Rescheduled":
			c.Rescheduled = n
		}
		c.Total += n
	}
	return c, rows.Err()
}

func (a *Analytics) cancellationRate(from, to string) (float64, error) {
	c, err := a.AppointmentStatusCounts(from, to)
	if err != nil || c.Total == 0 {
		return 0, err
	}
	return float64(c.Cancelled) / float64(c.Total), nil
}

func (a *Analytics) rescheduleRate(from, to string) (float64, error) {
	c, err := a.AppointmentStatusCounts(from, to)
	if err != nil || c.Total == 0 {
		return 0, err
	}
	return float64(c.Rescheduled) / float64(c.Total), nil
}

func (a *Analytics) upcomingCount(days int) (float64, error) {
	now := ClinicNow()
	start := DateFrom(now)
	end := DateFrom(now.AddDate(0, 0, days))
	var n float64
	err := RDB.QueryRow(`
		SELECT COUNT(*) FROM appointments
		WHERE status NOT IN ('Cancelled','Rescheduled')
		AND start_time >= ? AND start_time < ?
	`, start, end).Scan(&n)
	return n, err
}

type AppointmentDistribution struct {
	ByWeekday [7]int  `json:"byWeekday"`
	ByHour    [24]int `json:"byHour"`
}

// AppointmentDistribution returns booked appointment counts bucketed by weekday
// (0=Sunday) and hour. NOTE: buckets are computed in UTC, not clinic-local, so
// hour-of-day is offset from Beirut wall-clock. Good enough for spotting relative
// peaks, but refine if exact local hours are needed.
func (a *Analytics) AppointmentDistribution(from, to string) (AppointmentDistribution, error) {
	var d AppointmentDistribution
	wd, err := a.labelCounts(`
		SELECT CAST(strftime('%w', start_time) AS INT), COUNT(*)
		FROM appointments
		WHERE status NOT IN ('Cancelled','Rescheduled')
		AND start_time >= ? AND start_time < ?
		GROUP BY 1`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return d, err
	}
	for _, lc := range wd {
		if i := atoiSafe(lc.Label); i >= 0 && i < 7 {
			d.ByWeekday[i] = lc.Count
		}
	}
	hr, err := a.labelCounts(`
		SELECT CAST(strftime('%H', start_time) AS INT), COUNT(*)
		FROM appointments
		WHERE status NOT IN ('Cancelled','Rescheduled')
		AND start_time >= ? AND start_time < ?
		GROUP BY 1`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return d, err
	}
	for _, lc := range hr {
		if i := atoiSafe(lc.Label); i >= 0 && i < 24 {
			d.ByHour[i] = lc.Count
		}
	}
	return d, nil
}

type RoomUtilization struct {
	RoomID       string  `json:"roomId"`
	Name         string  `json:"name"`
	Hours        float64 `json:"hours"`
	Appointments int     `json:"appointments"`
}

// RoomUtilization returns booked appointment-hours per room over the range.
// There is no modeled room-capacity, so this is absolute booked hours rather
// than a utilization percentage.
func (a *Analytics) RoomUtilization(from, to string) ([]RoomUtilization, error) {
	rows, err := RDB.Query(`
		SELECT r.id, r.name,
			COALESCE(SUM((julianday(a.end_time) - julianday(a.start_time)) * 24), 0) AS hours,
			COUNT(*) AS appts
		FROM appointments a
		JOIN rooms r ON r.id = a.room_id
		WHERE a.status NOT IN ('Cancelled','Rescheduled')
		AND a.start_time >= ? AND a.start_time < ?
		GROUP BY r.id, r.name
		ORDER BY hours DESC
	`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RoomUtilization{}
	for rows.Next() {
		var r RoomUtilization
		if err := rows.Scan(&r.RoomID, &r.Name, &r.Hours, &r.Appointments); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Staff performance

type StaffPerformance struct {
	EmployeeID string `json:"employeeId"`
	Name       string `json:"name"`
	Procedures int    `json:"procedures"`
}

// StaffPerformance ranks practitioners by procedures performed on completed
// appointments in the range. (Revenue-per-practitioner isn't derivable: invoice
// items don't link back to the appointment_procedure that produced them.)
func (a *Analytics) StaffPerformance(from, to string) ([]StaffPerformance, error) {
	rows, err := RDB.Query(`
		SELECT e.id, e.first_name || ' ' || e.last_name, COUNT(*) AS c
		FROM appointment_procedures ap
		JOIN appointments a ON a.id = ap.appointment_id
		JOIN employees e ON e.id = ap.assigned_to_id
		WHERE a.status = 'Completed'
		AND a.start_time >= ? AND a.start_time < ?
		AND ap.assigned_to_id IS NOT NULL AND ap.assigned_to_id != ''
		GROUP BY e.id, e.first_name, e.last_name
		ORDER BY c DESC
	`, RangeStart(from), RangeEnd(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StaffPerformance{}
	for rows.Next() {
		var s StaffPerformance
		if err := rows.Scan(&s.EmployeeID, &s.Name, &s.Procedures); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Inventory

type InventoryAnalytics struct {
	LowStock        float64 `json:"lowStock"`
	TotalStockValue float64 `json:"totalStockValue"`
}

func (a *Analytics) Inventory() (InventoryAnalytics, error) {
	var inv InventoryAnalytics
	n, err := a.LowStockCount()
	if err != nil {
		return inv, err
	}
	inv.LowStock = float64(n)
	if inv.TotalStockValue, err = a.totalStockValue(); err != nil {
		return inv, err
	}
	return inv, nil
}

func (a *Analytics) totalStockValue() (float64, error) {
	var v float64
	err := RDB.QueryRow(`
		SELECT COALESCE(SUM(p.quantity * (
			SELECT pp.price FROM product_prices pp
			WHERE pp.product_id = p.id AND pp.is_active = 1
			ORDER BY pp.created_at DESC LIMIT 1
		)), 0)
		FROM products p
	`).Scan(&v)
	return v, err
}

type TopProduct struct {
	ProductID string `json:"productId"`
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
}

// TopProducts ranks products by quantity sold on client invoices in the range.
func (a *Analytics) TopProducts(from, to string, limit int) ([]TopProduct, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := RDB.Query(`
		SELECT ii.item_id, pd.name, COALESCE(SUM(ii.quantity), 0) AS q
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		JOIN products pd ON pd.id = ii.item_id
		WHERE ii.item_type = 'product' AND tb.entity_type = 'patient'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''
		GROUP BY ii.item_id, pd.name
		ORDER BY q DESC LIMIT ?
	`, RangeStart(from), RangeEnd(to), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TopProduct{}
	for rows.Next() {
		var p TopProduct
		if err := rows.Scan(&p.ProductID, &p.Name, &p.Quantity); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Full report (drives the PDF; mirrors the dashboard)

type AnalyticsReport struct {
	Money              MoneyAnalytics
	Patients           PatientsAnalytics
	Operations         OperationsAnalytics
	Inventory          InventoryAnalytics
	Demographics       Demographics
	TopProcedures      []TopProcedure
	TopProducts        []TopProduct
	StaffPerformance   []StaffPerformance
	ReferralSources    []ReferralSource
	RoomUtilization    []RoomUtilization
	TodaysAppointments AppointmentList
	RecentTransactions BalanceTransactionList
}

// Report assembles every dashboard section for the range: the single source
// the analytics PDF renders from.
func (a *Analytics) Report(from, to string) (AnalyticsReport, error) {
	var r AnalyticsReport
	var err error
	if r.Money, err = a.Money(from, to); err != nil {
		return r, err
	}
	if r.Patients, err = a.Patients(from, to); err != nil {
		return r, err
	}
	if r.Operations, err = a.Operations(from, to); err != nil {
		return r, err
	}
	if r.Inventory, err = a.Inventory(); err != nil {
		return r, err
	}
	if r.Demographics, err = a.Demographics(5); err != nil {
		return r, err
	}
	if r.TopProcedures, err = a.TopProcedures(from, to, 10, "revenue"); err != nil {
		return r, err
	}
	if r.TopProducts, err = a.TopProducts(from, to, 10); err != nil {
		return r, err
	}
	if r.StaffPerformance, err = a.StaffPerformance(from, to); err != nil {
		return r, err
	}
	if r.ReferralSources, err = a.ReferralSources(from, to); err != nil {
		return r, err
	}
	if r.RoomUtilization, err = a.RoomUtilization(from, to); err != nil {
		return r, err
	}
	if r.TodaysAppointments, err = a.RecentAppointmentsToday(8); err != nil {
		return r, err
	}
	if r.RecentTransactions, err = a.RecentTransactions(8); err != nil {
		return r, err
	}
	return r, nil
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	if s == "" {
		return -1
	}
	return n
}
