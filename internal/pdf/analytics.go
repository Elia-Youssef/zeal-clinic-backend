package pdf

import (
	"clinic-api/internal/assets"
	"clinic-api/internal/database/store"
	"fmt"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// Brand palette: sRGB conversions of the frontend's oklch tokens. The PDF
// keeps the brand colors but is structured for print legibility: bordered KPI
// cards, filled table headers, zebra rows, and full cell grids so the eye can
// track across a row and each table reads as a distinct block.
var (
	clrInk       = &props.Color{Red: 21, Green: 24, Blue: 27}    // foreground / values
	clrPrimary   = &props.Color{Red: 26, Green: 31, Blue: 35}    // table headers, section accent
	clrOnPrimary = &props.Color{Red: 250, Green: 251, Blue: 251} // text on primary
	clrMutedFg   = &props.Color{Red: 103, Green: 120, Blue: 124} // labels
	clrBorder    = &props.Color{Red: 215, Green: 221, Blue: 223} // grid lines
	clrTile      = &props.Color{Red: 243, Green: 245, Blue: 245} // card / section-bar fill
	clrZebra     = &props.Color{Red: 247, Green: 248, Blue: 249} // alternate row
	clrWhite     = &props.Color{Red: 255, Green: 255, Blue: 255}
	clrAccent    = &props.Color{Red: 86, Green: 102, Blue: 110} // section accent tab
	clrPositive  = &props.Color{Red: 34, Green: 144, Blue: 97}
	clrNegative  = &props.Color{Red: 192, Green: 69, Blue: 59}
)

// GenerateAnalyticsReport writes the print-oriented analytics PDF and returns
// its path. Each section is buffered and emitted as a unit so a page break
// never separates a section title from its body (see keep).
func GenerateAnalyticsReport(data *store.AnalyticsReport, from, to string) (string, error) {
	if data == nil {
		return "", fmt.Errorf("report is empty")
	}
	firstDay, lastDay, err := rangeDays(from, to)
	if err != nil {
		return "", err
	}

	cfg := config.NewBuilder().
		WithMaxGridSize(100).
		WithOrientation(orientation.Vertical).
		WithLeftMargin(12).
		WithRightMargin(12).
		WithTopMargin(12).
		WithBottomMargin(12).
		WithPageNumber(props.PageNumber{
			Pattern: "Zeal Clinic — Analytics      {current} / {total}",
			Place:   props.RightBottom,
			Size:    8,
			Color:   clrMutedFg,
		}).
		Build()
	m := maroto.New(cfg)

	addMasthead(m, firstDay, lastDay)
	keep(m, func(s rowSink) { addMoneySection(s, data.Money) })
	keep(m, func(s rowSink) { addPatientsSection(s, data.Patients) })
	keep(m, func(s rowSink) { addOperationsSection(s, data.Operations) })
	keep(m, func(s rowSink) { addAppointmentStatus(s, data.Operations.StatusBreakdown) })
	keep(m, func(s rowSink) { addInventorySection(s, data.Inventory) })
	keep(m, func(s rowSink) {
		addTable(s, "Revenue Mix (USD)", revenueMixSpecs, revenueMixRows(data.Money), "No revenue in range")
	})
	keep(m, func(s rowSink) {
		addTable(s, "Payment Methods (USD)", revenueMixSpecs, paymentMixRows(data.Money), "No payments in range")
	})
	keep(m, func(s rowSink) {
		addTable(s, "Patients by Gender", demoSpecs("Gender"), lcRows(data.Demographics.Gender), "No data")
	})
	keep(m, func(s rowSink) {
		addTable(s, "Patients by Age", demoSpecs("Age band"), lcRows(data.Demographics.AgeBands), "No data")
	})
	keep(m, func(s rowSink) {
		addTable(s, "Top Cities", demoSpecs("City"), lcRows(data.Demographics.TopCities), "No data")
	})
	keep(m, func(s rowSink) { addReferralSources(s, data.ReferralSources) })
	keep(m, func(s rowSink) { addStaffPerformance(s, data.StaffPerformance) })
	keep(m, func(s rowSink) { addTopProcedures(s, data.TopProcedures) })
	keep(m, func(s rowSink) { addTopProducts(s, data.TopProducts) })
	keep(m, func(s rowSink) { addRoomUtilization(s, data.RoomUtilization) })
	keep(m, func(s rowSink) { addTodaysAppointments(s, data.TodaysAppointments) })
	keep(m, func(s rowSink) { addRecentTransactions(s, data.RecentTransactions) })

	return save(m, fmt.Sprintf("analytics-report-%s-%s", firstDay, lastDay))
}

// keep-together pagination

// rowSink is the subset of core.Maroto the section builders need. A rowBuf
// implements it to capture a section's rows so they can be emitted as a unit.
type rowSink interface {
	AddRow(rowHeight float64, cols ...core.Col) core.Row
}

type rowBuf struct {
	rows   []core.Row
	height float64
}

func (b *rowBuf) AddRow(h float64, cols ...core.Col) core.Row {
	r := row.New(h).Add(cols...)
	b.rows = append(b.rows, r)
	b.height += h
	return r
}

// keep builds a section into a buffer, then emits it so it isn't split across
// a page boundary: if the whole block fits on the current page it flows
// normally; otherwise it is pushed to a fresh page (AddPages flushes the
// current page first), keeping the title with its body.
func keep(m core.Maroto, build func(rowSink)) {
	b := &rowBuf{}
	build(b)
	if len(b.rows) == 0 {
		return
	}
	if m.FitlnCurrentPage(b.height) {
		m.AddRows(b.rows...)
		return
	}
	var p core.Page
	if cfg := m.GetCurrentConfig(); cfg != nil && cfg.PageNumber != nil {
		p = page.New(*cfg.PageNumber)
	} else {
		p = page.New()
	}
	p.Add(b.rows...)
	m.AddPages(p)
}

// masthead

func addMasthead(m core.Maroto, firstDay, lastDay string) {
	brand := []core.Component{
		text.New("Zeal Clinic", props.Text{Size: 16, Style: fontstyle.Bold, Color: clrInk, Top: 2}),
		text.New("Analytics Report", props.Text{Size: 9.5, Color: clrMutedFg, Top: 9}),
	}
	meta := []core.Component{
		text.New(fmt.Sprintf("%s  —  %s", dayHeader(firstDay), dayHeader(lastDay)),
			props.Text{Size: 9.5, Style: fontstyle.Bold, Color: clrInk, Align: align.Right, Right: 1, Top: 3}),
		text.New(fmt.Sprintf("Generated %s", clinicDate(store.DateNow())),
			props.Text{Size: 8, Color: clrMutedFg, Align: align.Right, Right: 1, Top: 9}),
	}

	if logo := assets.InvoiceLogo(); len(logo) > 0 {
		m.AddRow(15,
			image.NewFromBytesCol(15, logo, extension.Png, props.Rect{Percent: 92, Center: true}),
			col.New(51).Add(brand...),
			col.New(34).Add(meta...),
		)
	} else {
		m.AddRow(15, col.New(66).Add(brand...), col.New(34).Add(meta...))
	}
	m.AddRow(2, col.New(100).WithStyle(&props.Cell{BorderType: border.Bottom, BorderColor: clrPrimary, BorderThickness: 0.4}))
}

// KPI sections

func addMoneySection(s rowSink, d store.MoneyAnalytics) {
	sectionBar(s, "Financial")
	kpiGrid(s,
		kpi("Revenue", money(d.Revenue.Value), d.Revenue.Change, true, true),
		kpi("Expenses", money(d.Expenses.Value), d.Expenses.Change, true, false),
		kpi("Net Profit", money(d.NetProfit.Value), d.NetProfit.Change, true, true),
		kpi("Avg Invoice", money(d.AvgInvoice.Value), d.AvgInvoice.Change, true, true),
		kpi("Discounts", money(d.Discounts.Value), d.Discounts.Change, true, false),
		kpi("Refunds", money(d.Refunds.Value), d.Refunds.Change, true, false),
		kpi("Write-offs", money(d.WriteOffs.Value), d.WriteOffs.Change, true, false),
		kpi("Receivables", money(d.Receivables), 0, false, true),
		kpi("Payables", money(d.Payables), 0, false, true),
		kpi("Gift Card Liability", money(d.GiftCardLiability), 0, false, true),
	)
}

func addPatientsSection(s rowSink, d store.PatientsAnalytics) {
	sectionBar(s, "Patients")
	kpiGrid(s,
		kpi("New Patients", fmtCount(d.NewPatients.Value), d.NewPatients.Change, true, true),
		kpi("Returning", fmtCount(d.ReturningPatients.Value), d.ReturningPatients.Change, true, true),
		kpi("Repeat Rate", fmtPct(d.RepeatRate.Value), d.RepeatRate.Change, true, true),
		kpi("Active (180d)", fmtCount(d.ActivePatients), 0, false, true),
	)
}

func addOperationsSection(s rowSink, d store.OperationsAnalytics) {
	sectionBar(s, "Operations")
	kpiGrid(s,
		kpi("Appointments", fmtCount(d.Appointments.Value), d.Appointments.Change, true, true),
		kpi("Procedures", fmtCount(d.ProceduresPerformed.Value), d.ProceduresPerformed.Change, true, true),
		kpi("Cancellation Rate", fmtPct(d.CancellationRate.Value), d.CancellationRate.Change, true, false),
		kpi("Reschedule Rate", fmtPct(d.RescheduleRate.Value), d.RescheduleRate.Change, true, false),
		kpi("Upcoming (7d)", fmtCount(d.Upcoming7Days), 0, false, true),
	)
}

func addInventorySection(s rowSink, d store.InventoryAnalytics) {
	sectionBar(s, "Inventory")
	kpiGrid(s,
		kpi("Low Stock", fmtCount(d.LowStock), 0, false, true),
		kpi("Stock Value", money(d.TotalStockValue), 0, false, true),
	)
}

// status & mixes

func addAppointmentStatus(s rowSink, st store.AppointmentStatusCounts) {
	sectionBar(s, "Appointment Status")
	headerRow(s, colSpec{75, "Status", align.Left}, colSpec{25, "Count", align.Right})
	rows := []struct {
		label string
		count int
	}{
		{"Scheduled", st.Scheduled}, {"In-Progress", st.InProgress}, {"Completed", st.Completed},
		{"Cancelled", st.Cancelled}, {"Rescheduled", st.Rescheduled},
	}
	for i, r := range rows {
		bg := zebra(i)
		s.AddRow(6.5,
			gridText(75, r.label, props.Text{Size: 8.4, Color: clrInk, Left: 2, Top: 1.1}, bg),
			gridText(25, fmt.Sprintf("%d", r.count), props.Text{Size: 8.4, Color: clrInk, Align: align.Right, Right: 2, Top: 1.1}, bg),
		)
	}
	s.AddRow(7,
		gridText(75, "Total", props.Text{Size: 8.6, Style: fontstyle.Bold, Color: clrInk, Left: 2, Top: 1.3}, clrTile),
		gridText(25, fmt.Sprintf("%d", st.Total), props.Text{Size: 8.6, Style: fontstyle.Bold, Color: clrInk, Align: align.Right, Right: 2, Top: 1.3}, clrTile),
	)
}

var revenueMixSpecs = []colSpec{{70, "Category", align.Left}, {30, "Amount", align.Right}}

func demoSpecs(label string) []colSpec {
	return []colSpec{{70, label, align.Left}, {30, "Patients", align.Right}}
}

func revenueMixRows(d store.MoneyAnalytics) [][]string {
	return [][]string{
		{"Procedures", money(d.RevenueMix.Procedures)},
		{"Products", money(d.RevenueMix.Products)},
		{"Gifts", money(d.RevenueMix.Gifts)},
		{"Other", money(d.RevenueMix.Other)},
	}
}

func paymentMixRows(d store.MoneyAnalytics) [][]string {
	out := make([][]string, 0, len(d.PaymentMix))
	for _, p := range d.PaymentMix {
		out = append(out, []string{titleize(p.Method), money(p.Amount)})
	}
	return out
}

// ranked tables

func addReferralSources(s rowSink, items []store.ReferralSource) {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{it.Source, fmt.Sprintf("%d", it.Count)})
	}
	addTable(s, "Referral Sources",
		[]colSpec{{75, "Source", align.Left}, {25, "Patients", align.Right}}, rows, "No new patients in range")
}

func addStaffPerformance(s rowSink, items []store.StaffPerformance) {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{it.Name, fmt.Sprintf("%d", it.Procedures)})
	}
	addTable(s, "Staff Performance",
		[]colSpec{{75, "Practitioner", align.Left}, {25, "Procedures", align.Right}}, rows, "No staff activity in this range")
}

func addTopProcedures(s rowSink, items []store.TopProcedure) {
	rows := make([][]string, 0, len(items))
	for i, it := range items {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), it.Name, fmt.Sprintf("%d", it.Count), money(it.Amount)})
	}
	addTable(s, "Top Procedures (by revenue)",
		[]colSpec{{8, "#", align.Left}, {52, "Procedure", align.Left}, {16, "Count", align.Right}, {24, "Revenue", align.Right}},
		rows, "No procedures in range")
}

func addTopProducts(s rowSink, items []store.TopProduct) {
	rows := make([][]string, 0, len(items))
	for i, it := range items {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), it.Name, fmt.Sprintf("%d", it.Quantity)})
	}
	addTable(s, "Top Products (by quantity)",
		[]colSpec{{8, "#", align.Left}, {72, "Product", align.Left}, {20, "Qty", align.Right}}, rows, "No products sold in range")
}

func addRoomUtilization(s rowSink, items []store.RoomUtilization) {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{it.Name, fmt.Sprintf("%.1fh", it.Hours), fmt.Sprintf("%d", it.Appointments)})
	}
	addTable(s, "Room Utilization",
		[]colSpec{{60, "Room", align.Left}, {20, "Booked Hours", align.Right}, {20, "Appts", align.Right}}, rows, "No bookings in range")
}

func addTodaysAppointments(s rowSink, items store.AppointmentList) {
	sectionBar(s, "Today's Appointments")
	headerRow(s, colSpec{52, "Patient", align.Left}, colSpec{18, "Time", align.Left}, colSpec{30, "Status", align.Right})
	if len(items) == 0 {
		emptyRow(s, "No appointments today")
		return
	}
	for i, a := range items {
		bg := zebra(i)
		s.AddRow(6.5,
			gridText(52, a.PatientName, props.Text{Size: 8.2, Color: clrInk, Left: 2, Top: 1.1}, bg),
			gridText(18, beirutHour(a.StartTime), props.Text{Size: 8.2, Color: clrMutedFg, Left: 1, Top: 1.1}, bg),
			gridText(30, a.Status, props.Text{Size: 8.2, Color: clrInk, Align: align.Right, Right: 2, Top: 1.1}, bg),
		)
	}
}

func addRecentTransactions(s rowSink, items store.BalanceTransactionList) {
	sectionBar(s, "Recent Transactions")
	headerRow(s, colSpec{75, "Description", align.Left}, colSpec{25, "Amount", align.Right})
	if len(items) == 0 {
		emptyRow(s, "No recent transactions")
		return
	}
	for i, t := range items {
		bg := zebra(i)
		desc := t.Description
		if desc == "" {
			desc = t.FromEntityName
		}
		sub := titleize(t.TransactionType)
		if t.TransactionMethod != "" {
			sub += " · " + titleize(t.TransactionMethod)
		}
		descCol := col.New(75).Add(
			text.New(desc, props.Text{Size: 8.2, Color: clrInk, Left: 2, Top: 1}),
			text.New(sub, props.Text{Size: 6.8, Color: clrMutedFg, Left: 2, Top: 5.2}),
		).WithStyle(&props.Cell{BackgroundColor: bg, BorderType: border.Full, BorderColor: clrBorder, BorderThickness: 0.1})
		amountCol := gridText(25, "+"+money(t.Amount),
			props.Text{Size: 8.6, Style: fontstyle.Bold, Color: clrPositive, Align: align.Right, Right: 2, Top: 2.2}, bg)
		s.AddRow(9, descCol, amountCol)
	}
}

// building blocks

// sectionBar is a left accent tab + filled bar carrying the section title,
// the primary visual divider between blocks.
func sectionBar(s rowSink, title string) {
	s.AddRow(6)
	s.AddRow(9,
		col.New(2).WithStyle(&props.Cell{BackgroundColor: clrAccent}),
		text.NewCol(98, title, props.Text{Size: 10.5, Style: fontstyle.Bold, Color: clrPrimary, Left: 3, Top: 2.2}).
			WithStyle(&props.Cell{BackgroundColor: clrTile}),
	)
	s.AddRow(2)
}

// kpi describes one tile; kpiGrid lays them out 4-per-row as bordered cards.
type kpiTile struct {
	label, value string
	change       float64
	hasDelta     bool
	goodWhenUp   bool
}

func kpi(label, value string, change float64, hasDelta, goodWhenUp bool) kpiTile {
	return kpiTile{label, value, change, hasDelta, goodWhenUp}
}

func kpiGrid(s rowSink, tiles ...kpiTile) {
	const perRow = 4
	for i := 0; i < len(tiles); i += perRow {
		cols := make([]core.Col, 0, perRow)
		used := 0
		for j := i; j < i+perRow && j < len(tiles); j++ {
			cols = append(cols, kpiCard(25, tiles[j]))
			used += 25
		}
		if used < 100 {
			cols = append(cols, col.New(100-used))
		}
		s.AddRow(18, cols...)
	}
}

func kpiCard(size int, t kpiTile) core.Col {
	comps := []core.Component{
		text.New(t.label, props.Text{Size: 7.2, Color: clrMutedFg, Left: 2.5, Top: 2.5}),
		text.New(t.value, props.Text{Size: 13, Style: fontstyle.Bold, Color: clrInk, Left: 2.5, Top: 7.5}),
	}
	if t.hasDelta {
		comps = append(comps, text.New(deltaText(t.change),
			props.Text{Size: 7, Style: fontstyle.Bold, Color: deltaColor(t.change, t.goodWhenUp), Left: 2.5, Top: 13}))
	}
	return col.New(size).Add(comps...).
		WithStyle(&props.Cell{BackgroundColor: clrTile, BorderType: border.Full, BorderColor: clrBorder, BorderThickness: 0.1})
}

type colSpec struct {
	size  int
	title string
	align align.Type
}

// addTable draws a section bar, a filled header row, then zebra-striped,
// fully-bordered body rows.
func addTable(s rowSink, title string, specs []colSpec, rows [][]string, emptyMsg string) {
	sectionBar(s, title)
	headerRow(s, specs...)
	if len(rows) == 0 {
		emptyRow(s, emptyMsg)
		return
	}
	for i, r := range rows {
		bg := zebra(i)
		cells := make([]core.Col, 0, len(specs))
		for j, spec := range specs {
			cells = append(cells, gridText(spec.size, r[j], bodyTextProps(spec.align), bg))
		}
		s.AddRow(6.5, cells...)
	}
}

func headerRow(s rowSink, specs ...colSpec) {
	cells := make([]core.Col, 0, len(specs))
	for _, spec := range specs {
		c := text.NewCol(spec.size, spec.title, props.Text{Size: 8, Style: fontstyle.Bold, Color: clrOnPrimary, Align: spec.align, Left: padLeft(spec.align), Right: padRight(spec.align), Top: 1.4})
		c.WithStyle(&props.Cell{BackgroundColor: clrPrimary, BorderType: border.Full, BorderColor: clrPrimary, BorderThickness: 0.1})
		cells = append(cells, c)
	}
	s.AddRow(7, cells...)
}

func bodyTextProps(a align.Type) props.Text {
	return props.Text{Size: 8.2, Color: clrInk, Align: a, Left: padLeft(a), Right: padRight(a), Top: 1.1}
}

func padLeft(a align.Type) float64 {
	if a == align.Left {
		return 2
	}
	return 0
}

func padRight(a align.Type) float64 {
	if a == align.Right {
		return 2
	}
	return 0
}

func gridText(size int, value string, p props.Text, bg *props.Color) core.Col {
	c := text.NewCol(size, value, p)
	c.WithStyle(&props.Cell{BackgroundColor: bg, BorderType: border.Full, BorderColor: clrBorder, BorderThickness: 0.1})
	return c
}

func emptyRow(s rowSink, msg string) {
	gridless := props.Text{Size: 8, Style: fontstyle.Italic, Color: clrMutedFg, Align: align.Center, Top: 1.3}
	s.AddRow(6.5, gridText(100, msg, gridless, clrWhite))
}

func zebra(i int) *props.Color {
	if i%2 == 1 {
		return clrZebra
	}
	return clrWhite
}

// formatting

func deltaText(change float64) string {
	if change == 0 {
		return "+0.0%"
	}
	sign := "+"
	if change < 0 {
		sign = "-"
		change = -change
	}
	return fmt.Sprintf("%s%.1f%%", sign, change*100)
}

func deltaColor(change float64, goodWhenUp bool) *props.Color {
	if change == 0 {
		return clrMutedFg
	}
	if (change > 0) == goodWhenUp {
		return clrPositive
	}
	return clrNegative
}

func fmtCount(v float64) string { return fmt.Sprintf("%.0f", v) }

func fmtPct(rate float64) string { return fmt.Sprintf("%.1f%%", rate*100) }

func titleize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 32
	}
	return string(r)
}

func lcRows(in []store.LabelCount) [][]string {
	rows := make([][]string, 0, len(in))
	for _, lc := range in {
		rows = append(rows, []string{lc.Label, fmt.Sprintf("%d", lc.Count)})
	}
	return rows
}
