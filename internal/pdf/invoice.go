package pdf

import (
	"clinic-api/internal/assets"
	"clinic-api/internal/database/store"
	"fmt"
	"strings"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// GenerateInvoice writes an invoice PDF and returns its absolute path.
func GenerateInvoice(inv *store.Invoice) (string, error) {
	if inv == nil || inv.ID == "" {
		return "", fmt.Errorf("invoice is empty")
	}
	var patient store.Patient
	if inv.ToEntityID != "" {
		_ = patient.GetByID(inv.ToEntityID)
	}
	patientName := patientDisplayName(patient, inv.ToEntityName)
	patientAddress := joinNonEmpty(", ", patient.Country.Name, patient.City.Name, patient.Address)
	dateStr := clinicDate(inv.CreatedAt)

	cfg := config.NewBuilder().
		WithMaxGridSize(24).
		WithLeftMargin(12).
		WithRightMargin(12).
		WithTopMargin(12).
		WithBottomMargin(12).
		WithPageNumber(props.PageNumber{
			Pattern: fmt.Sprintf("Zeal Clinic — Invoice #%08d      {current} / {total}", inv.InvoiceNumber),
			Place:   props.RightBottom,
			Size:    8,
			Color:   clrMutedFg,
		}).
		Build()
	m := maroto.New(cfg)

	// Header (logo, patient/invoice info grid, items column header) repeats on
	// every page via RegisterHeader.
	hdr := &rowBuf{}
	if logo := assets.InvoiceLogo(); len(logo) > 0 {
		hdr.AddRow(26, image.NewFromBytesCol(24, logo, extension.Png, props.Rect{Percent: 78, Center: true}))
	} else {
		hdr.AddRow(14, text.NewCol(24, "Zeal Clinic", props.Text{Size: 18, Style: fontstyle.Bold, Color: clrInk, Align: align.Center, Top: 4}))
	}
	hdr.AddRow(2, col.New(24).WithStyle(&props.Cell{BorderType: border.Bottom, BorderColor: clrPrimary, BorderThickness: 0.4}))
	hdr.AddRow(3)

	type kv struct{ label, value string }
	left := []kv{{"Patient", patientName}, {"Address", patientAddress}, {"Tel", patient.Contact}}
	right := []kv{{"Invoice #", fmt.Sprintf("%08d", inv.InvoiceNumber)}, {"Currency", "USD"}, {"Date", dateStr}}
	for i := range left {
		hdr.AddRow(6,
			boldCell(4, left[i].label, align.Left),
			bodyCell(11, left[i].value, align.Left),
			col.New(1),
			boldCell(3, right[i].label, align.Left),
			bodyCell(5, right[i].value, align.Left),
		)
	}

	titleBar(hdr, 24, "Items")
	hdr.AddRow(8,
		headerCell(13, "Description", align.Left),
		headerCell(3, "Qty", align.Right),
		headerCell(4, "U.Price", align.Right),
		headerCell(4, "Total", align.Right),
	)
	if err := m.RegisterHeader(hdr.rows...); err != nil {
		return "", err
	}

	for _, it := range inv.Items {
		unit := 0.0
		if it.Quantity > 0 {
			unit = it.Amount / float64(it.Quantity)
		}
		m.AddRow(7,
			bodyCell(13, invoiceItemName(it), align.Left),
			bodyCell(3, fmt.Sprintf("%d", it.Quantity), align.Right),
			bodyCell(4, money(unit), align.Right),
			bodyCell(4, money(it.Amount), align.Right),
		)
	}

	// Totals: amount in words on the left, figures stacked on the right. The
	// value column is wide because the LBP equivalent can run into billions.
	words := "USD " + numberToWords(int64(inv.FinalAmount)) + " Only."
	m.AddRow(4)
	m.AddRow(7,
		text.NewCol(15, words, props.Text{Size: 8.5, Style: fontstyle.Italic, Color: clrMutedFg, Top: 1.5}),
		boldCell(4, "Gross Total", align.Left),
		boldCell(5, money(inv.Amount), align.Right),
	)
	if inv.DiscountValue > 0 {
		m.AddRow(7,
			text.NewCol(15, "", props.Text{}),
			boldCell(4, "Discount", align.Left),
			boldCell(5, "-"+money(inv.DiscountValue), align.Right),
		)
	}
	m.AddRow(8,
		text.NewCol(15, "", props.Text{}),
		emphCell(4, "Net", align.Left),
		emphCell(5, money(inv.FinalAmount), align.Right),
	)
	// LBP equivalent at the current table rate (the rate may change over time).
	if rate := store.LBPRate(); rate > 0 {
		m.AddRow(7,
			text.NewCol(15, "", props.Text{}),
			boldCell(4, "Equivalent LBP", align.Left),
			boldCell(5, money(inv.FinalAmount*rate), align.Right),
		)
	}

	return save(m, fmt.Sprintf("invoice-%d", inv.InvoiceNumber))
}

func patientDisplayName(patient store.Patient, fallback string) string {
	name := strings.Join(compactNonEmpty(patient.FirstName, patient.MiddleName, patient.LastName), " ")
	if name != "" {
		return name
	}
	return fallback
}

func invoiceItemName(it store.InvoiceItem) string {
	if strings.TrimSpace(it.ItemName) != "" {
		return it.ItemName
	}
	if strings.TrimSpace(it.Notes) != "" {
		return it.Notes
	}
	if it.ItemType != "" {
		return strings.Title(it.ItemType)
	}
	return "Other"
}

func joinNonEmpty(sep string, parts ...string) string {
	return strings.Join(compactNonEmpty(parts...), sep)
}

func compactNonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
