package pdf

import (
	"clinic-api/internal/database/store"
	"fmt"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// GenerateRevenueReport writes a revenue report PDF and returns its path.
func GenerateRevenueReport(report *store.RevenueReport, from, to string) (string, error) {
	if report == nil {
		return "", fmt.Errorf("report is empty")
	}
	firstDay, lastDay, err := rangeFileDates(from, to)
	if err != nil {
		return "", err
	}

	m := newReportDoc("Revenue Report")

	hdr := &rowBuf{}
	masthead(hdr, 100, "Revenue Report", []string{
		fmt.Sprintf("%s — %s", reportRangeStart(from), reportRangeEnd(to)),
		"Currency: USD",
	})
	titleBar(hdr, 100, revenueReportSheet(report.Level)+" — by "+revenueReportAccumulate(report.Level))
	hdr.AddRow(8,
		headerCell(6, "Rank", align.Center),
		headerCell(50, reportNameHeader(report.Level), align.Left),
		headerCell(14, "Quantity", align.Right),
		headerCell(18, "Amount", align.Right),
		headerCell(12, "%", align.Right),
	)
	if err := m.RegisterHeader(hdr.rows...); err != nil {
		return "", err
	}

	for i, g := range report.Items {
		m.AddRow(7,
			bodyCell(6, fmt.Sprintf("%d", i+1), align.Center),
			bodyCell(50, g.EntityName, align.Left),
			bodyCell(14, reportQuantity(g.Quantity), align.Right),
			bodyCell(18, money(g.Amount), align.Right),
			bodyCell(12, fmt.Sprintf("%.2f", g.Percentage), align.Right),
		)
	}
	m.AddRow(8,
		emphCell(6, "", align.Center),
		emphCell(50, "Grand Total", align.Left),
		emphCell(14, reportQuantity(report.Totals.Quantity), align.Right),
		emphCell(18, money(report.Totals.Amount), align.Right),
		emphCell(12, "", align.Right),
	)

	return save(m, fmt.Sprintf("revenue-report-%s-%s", firstDay, lastDay))
}

// GenerateExpensesReport writes an expenses report PDF and returns its path.
func GenerateExpensesReport(report store.ExpensesReport, from, to string) (string, error) {
	firstDay, lastDay, err := rangeFileDates(from, to)
	if err != nil {
		return "", err
	}
	m := newReportDoc("Expenses Report")

	hdr := &rowBuf{}
	masthead(hdr, 100, "Expenses Report", []string{
		fmt.Sprintf("%s — %s", reportRangeStart(from), reportRangeEnd(to)),
		"Currency: USD",
	})
	titleBar(hdr, 100, "Clinic Expenses")
	hdr.AddRow(8,
		headerCell(12, "Date", align.Left),
		headerCell(20, "Supplier", align.Left),
		headerCell(26, "Description", align.Left),
		headerCell(10, "Quantity", align.Right),
		headerCell(12, "Unit Price", align.Right),
		headerCell(10, "Amount", align.Right),
		headerCell(10, "Remaining", align.Right),
	)
	if err := m.RegisterHeader(hdr.rows...); err != nil {
		return "", err
	}

	for _, r := range report.Rows {
		m.AddRow(7,
			bodyCell(12, clinicDate(r.Date), align.Left),
			bodyCell(20, r.Supplier, align.Left),
			bodyCell(26, r.Description, align.Left),
			bodyCell(10, reportQuantity(r.Quantity), align.Right),
			bodyCell(12, money(r.AmountPerUnit), align.Right),
			bodyCell(10, money(r.Amount), align.Right),
			bodyCell(10, money(r.RemainingBalance), align.Right),
		)
	}
	m.AddRow(8,
		emphCell(12, "", align.Left),
		emphCell(20, "", align.Left),
		emphCell(26, "Grand Total", align.Left),
		emphCell(10, "", align.Right),
		emphCell(12, "", align.Right),
		emphCell(10, money(report.Totals.Amount), align.Right),
		emphCell(10, money(report.Totals.Remaining), align.Right),
	)

	return save(m, fmt.Sprintf("expenses-report-%s-%s", firstDay, lastDay))
}

func newReportDoc(label string) core.Maroto {
	cfg := config.NewBuilder().
		WithMaxGridSize(100).
		WithOrientation(orientation.Horizontal).
		WithLeftMargin(8).
		WithRightMargin(8).
		WithTopMargin(8).
		WithBottomMargin(10).
		WithPageNumber(props.PageNumber{
			Pattern: "Zeal Clinic — " + label + "      {current} / {total}",
			Place:   props.RightBottom,
			Size:    8,
			Color:   clrMutedFg,
		}).
		Build()
	return maroto.New(cfg)
}

func revenueReportSheet(level string) string {
	switch level {
	case "product", "product-category":
		return "Clinic Products"
	case "discount":
		return "Clinic Discounts"
	case "kind", "all", "other":
		return "Clinic"
	default:
		return "Clinic Procedures"
	}
}

func revenueReportAccumulate(level string) string {
	switch level {
	case "product":
		return "Product"
	case "product-category":
		return "Product Category"
	case "procedure-category":
		return "Procedure Category"
	case "procedure-type":
		return "Procedure Type"
	case "kind":
		return "Kind"
	case "all":
		return "Item"
	case "other":
		return "Other"
	case "discount":
		return "Discount"
	default:
		return "Procedure"
	}
}

func reportNameHeader(level string) string {
	switch level {
	case "product", "product-category":
		return "Product Name"
	case "kind", "all":
		return "Name"
	case "other":
		return "Description"
	case "discount":
		return "Discount"
	default:
		return "Procedure Name"
	}
}

// reportRangeStart formats an inclusive lower range bound in clinic-local
// (Beirut) time as DD/MM/YYYY. Bounds arrive as either RFC3339 UTC instants or
// bare YYYY-MM-DD dates.
func reportRangeStart(v string) string {
	return clinicDate(store.Date(v))
}

// reportRangeEnd formats a range upper bound. RFC3339 bounds are exclusive
// half-open instants, so step back one second to land on the last included
// clinic-local day; bare YYYY-MM-DD bounds are already inclusive.
func reportRangeEnd(v string) string {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.Add(-time.Second).In(store.ClinicLocation()).Format("02/01/2006")
	}
	return clinicDate(store.Date(v))
}

func reportQuantity(v int) string {
	return fmt.Sprintf("%.2f", float64(v))
}
