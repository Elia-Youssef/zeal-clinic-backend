package pdf

import (
	"clinic-api/internal/database/store"
	"fmt"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// GenerateRevenueReport writes a revenue report PDF and returns its path.
func GenerateRevenueReport(report *store.RevenueReport, from, to, currencyID string) (string, error) {
	if report == nil {
		return "", fmt.Errorf("report is empty")
	}

	curLabel := currencyLabel(currencyID)

	cfg := config.NewBuilder().
		WithMaxGridSize(100).
		WithOrientation(orientation.Horizontal).
		WithLeftMargin(8).
		WithRightMargin(8).
		WithTopMargin(8).
		WithBottomMargin(8).
		WithPageNumber(props.PageNumber{
			Pattern: "page  {current} / {total}",
			Place:   props.RightTop,
			Size:    8.5,
			Style:   fontstyle.Bold,
		}).
		Build()
	m := maroto.New(cfg)

	if err := m.RegisterHeader(revenueReportHeaderRows(report, from, to, curLabel)...); err != nil {
		return "", err
	}

	rowTxt := props.Text{Size: 8.2, Left: 1, Top: 0.8}
	numTxt := props.Text{Size: 8.2, Align: align.Right, Right: 1, Top: 0.8}
	for i, g := range report.Items {
		m.AddRow(6,
			cellCol(6, fmt.Sprintf("%d", i+1), numTxt, border.Full),
			cellCol(50, g.EntityName, rowTxt, border.Full),
			cellCol(14, reportQuantity(g.Quantity), numTxt, border.Full),
			cellCol(18, money(g.Amount), numTxt, border.Full),
			cellCol(12, fmt.Sprintf("%.5f", g.Percentage), numTxt, border.Full),
		)
	}

	m.AddRow(7,
		cellCol(6, "", rowTxt, border.None),
		cellCol(50, "Grand Total", props.Text{Size: 10, Style: fontstyle.Bold, Align: align.Center, Top: 1}, border.Full),
		cellCol(14, reportQuantity(report.Totals.Quantity), props.Text{Size: 8.5, Style: fontstyle.Bold, Align: align.Right, Right: 1, Top: 1}, border.Full),
		cellCol(18, money(report.Totals.Amount), props.Text{Size: 8.5, Style: fontstyle.Bold, Align: align.Right, Right: 1, Top: 1}, border.Full),
		cellCol(12, "", rowTxt, border.None),
	)

	return save(m, "revenue-"+from+"-"+to)
}

// GenerateExpensesReport writes an expenses report PDF and returns its path.
func GenerateExpensesReport(rows []store.ExpenseRow, from, to, currencyID string) (string, error) {
	curLabel := currencyLabel(currencyID)

	cfg := config.NewBuilder().
		WithMaxGridSize(100).
		WithOrientation(orientation.Horizontal).
		WithLeftMargin(8).
		WithRightMargin(8).
		WithTopMargin(8).
		WithBottomMargin(8).
		WithPageNumber(props.PageNumber{
			Pattern: "page  {current} / {total}",
			Place:   props.RightTop,
			Size:    8.5,
			Style:   fontstyle.Bold,
		}).
		Build()
	m := maroto.New(cfg)

	if err := m.RegisterHeader(expensesReportHeaderRows(from, to, curLabel)...); err != nil {
		return "", err
	}

	var amountTotal, remainingTotal float64
	rowTxt := props.Text{Size: 8.2, Left: 1, Top: 0.8}
	numTxt := props.Text{Size: 8.2, Align: align.Right, Right: 1, Top: 0.8}
	for i, r := range rows {
		m.AddRow(6,
			cellCol(6, fmt.Sprintf("%d", i+1), numTxt, border.Full),
			cellCol(12, reportDate(r.Date.DateOnly()), rowTxt, border.Full),
			cellCol(20, r.Supplier, rowTxt, border.Full),
			cellCol(30, r.Description, rowTxt, border.Full),
			cellCol(10, reportQuantity(r.Quantity), numTxt, border.Full),
			cellCol(12, money(r.Amount), numTxt, border.Full),
			cellCol(10, money(r.RemainingBalance), numTxt, border.Full),
		)
		amountTotal += r.Amount
		remainingTotal += r.RemainingBalance
	}

	m.AddRow(7,
		cellCol(6, "", rowTxt, border.None),
		cellCol(12, "", rowTxt, border.None),
		cellCol(20, "", rowTxt, border.None),
		cellCol(30, "Grand Total", props.Text{Size: 10, Style: fontstyle.Bold, Align: align.Center, Top: 1}, border.Full),
		cellCol(10, "", rowTxt, border.Full),
		cellCol(12, money(amountTotal), props.Text{Size: 8.5, Style: fontstyle.Bold, Align: align.Right, Right: 1, Top: 1}, border.Full),
		cellCol(10, money(remainingTotal), props.Text{Size: 8.5, Style: fontstyle.Bold, Align: align.Right, Right: 1, Top: 1}, border.Full),
	)

	return save(m, "expenses-"+from+"-"+to)
}

func revenueReportHeaderRows(report *store.RevenueReport, from, to, curLabel string) []core.Row {
	sheet := revenueReportSheet(report.Level)
	if curLabel != "" {
		sheet += "  " + curLabel
	}

	headerTxt := props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Center, Top: 1.3}

	return []core.Row{
		row.New(7).Add(
			text.NewCol(10, "Sheet", props.Text{Size: 9, Style: fontstyle.Bold}),
			text.NewCol(20, sheet, props.Text{Size: 9}),
		),
		row.New(7).Add(
			text.NewCol(10, "Accumulate By", props.Text{Size: 9, Style: fontstyle.Bold}),
			text.NewCol(20, revenueReportAccumulate(report.Level), props.Text{Size: 9}),
			text.NewCol(55, ""),
			text.NewCol(15, fmt.Sprintf("%s - %s", reportDate(from), reportDate(to)), props.Text{Size: 9, Align: align.Right, Right: 1}),
		),
		row.New(5),
		row.New(9).Add(
			cellCol(6, "Rank", headerTxt, border.Full),
			cellCol(50, reportNameHeader(report.Level), headerTxt, border.Full),
			cellCol(14, "Quantity", headerTxt, border.Full),
			cellCol(18, "Amount", headerTxt, border.Full),
			cellCol(12, "%", headerTxt, border.Full),
		),
	}
}

func expensesReportHeaderRows(from, to, curLabel string) []core.Row {
	sheet := "Clinic expenses"
	if curLabel != "" {
		sheet += "  " + curLabel
	}
	headerTxt := props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Center, Top: 1.3}

	return []core.Row{
		row.New(7).Add(
			text.NewCol(10, "Sheet", props.Text{Size: 9, Style: fontstyle.Bold}),
			text.NewCol(20, sheet, props.Text{Size: 9}),
		),
		row.New(7).Add(
			text.NewCol(10, "Accumulate By", props.Text{Size: 9, Style: fontstyle.Bold}),
			text.NewCol(20, "Expense", props.Text{Size: 9}),
			text.NewCol(55, ""),
			text.NewCol(15, fmt.Sprintf("%s - %s", reportDate(from), reportDate(to)), props.Text{Size: 9, Align: align.Right, Right: 1}),
		),
		row.New(5),
		row.New(9).Add(
			cellCol(6, "Rank", headerTxt, border.Full),
			cellCol(12, "Date", headerTxt, border.Full),
			cellCol(20, "Supplier", headerTxt, border.Full),
			cellCol(30, "Description", headerTxt, border.Full),
			cellCol(10, "Quantity", headerTxt, border.Full),
			cellCol(12, "Amount", headerTxt, border.Full),
			cellCol(10, "Remaining", headerTxt, border.Full),
		),
	}
}

func revenueReportSheet(level string) string {
	switch level {
	case "product", "product-category":
		return "Clinic product"
	case "kind":
		return "Clinic"
	default:
		return "Clinic procedure"
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
	default:
		return "Procedure"
	}
}

func reportNameHeader(level string) string {
	switch level {
	case "product", "product-category":
		return "Product Name"
	case "kind":
		return "Name"
	default:
		return "Procedure Name"
	}
}

func reportDate(v string) string {
	t, err := time.Parse(store.DateFormat, v)
	if err != nil {
		return v
	}
	return t.Format("02/01/2006")
}

func reportQuantity(v int) string {
	return fmt.Sprintf("%.2f", float64(v))
}

func currencyLabel(currencyID string) string {
	if currencyID == "" {
		return ""
	}
	c := store.Currency{}
	if err := c.GetByID(currencyID); err != nil {
		return ""
	}
	if c.Code != "" {
		return c.Code
	}
	return c.Symbol
}
