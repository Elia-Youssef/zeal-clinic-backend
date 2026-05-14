package pdf

import (
	"clinic-api/client"
	"clinic-api/internal/database/store"
	"fmt"
	"io/fs"
	"strings"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

var logoBytes []byte

func loadLogo() []byte {
	if logoBytes != nil {
		return logoBytes
	}
	if data, err := fs.ReadFile(client.DistFS(), "zeal.png"); err == nil {
		logoBytes = data
	}
	return logoBytes
}

// GenerateInvoice writes an invoice PDF and returns its absolute path.
func GenerateInvoice(inv *store.Invoice) (string, error) {
	if inv == nil || inv.ID == "" {
		return "", fmt.Errorf("invoice is empty")
	}
	curLabel := invoiceCurrency(inv.CurrencyID)

	var patient store.Patient
	if inv.ToEntityID != "" {
		_ = patient.GetByID(inv.ToEntityID)
	}
	patientID := patient.ID
	if patientID == "" {
		patientID = inv.ToEntityID
	}
	patientName := patientDisplayName(patient, inv.ToEntityName)
	patientAddress := joinNonEmpty(", ", patient.Country.Name, patient.City.Name, patient.Address)

	dateStr := inv.CreatedAt.DateOnly()
	if t, err := inv.CreatedAt.Time(); err == nil {
		dateStr = t.Format("02/01/2006")
	}

	cfg := config.NewBuilder().
		WithMaxGridSize(24).
		WithLeftMargin(12).
		WithRightMargin(12).
		WithTopMargin(12).
		WithBottomMargin(12).
		Build()
	m := maroto.New(cfg)

	if logo := loadLogo(); len(logo) > 0 {
		m.AddRow(36,
			image.NewFromBytesCol(24, logo, extension.Png, props.Rect{Percent: 86, Center: true}),
		)
	} else {
		m.AddRow(36)
	}
	m.AddRow(5)

	leftRows := []invoiceInfoRow{
		{"Patient", patientName},
		{"Address", patientAddress},
		{"Tel", patient.Contact},
	}
	rightRows := []invoiceInfoRow{
		{"Invoice #", fmt.Sprintf("%08d", inv.InvoiceNumber)},
		{"Currency", curLabel},
		{"Date", dateStr},
	}

	labelTxt := props.Text{Size: 8.8, Style: fontstyle.Bold, Left: 1, Top: 1.2}
	valueTxt := props.Text{Size: 8.8, Left: 1, Top: 1.2}

	for i := range leftRows {
		addInvoiceInfoRow(m, i, len(leftRows)-1, leftRows[i], rightRows[i], labelTxt, valueTxt)
	}

	m.AddRow(7)
	headerTxt := props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Center, Top: 1.4}
	m.AddRow(8,
		cellCol(4, "Item No.", headerTxt, border.Full),
		cellCol(12, "Description", headerTxt, border.Full),
		cellCol(2, "Qty.", headerTxt, border.Full),
		cellCol(3, "U.Price", headerTxt, border.Full),
		cellCol(3, "Total", headerTxt, border.Full),
	)

	itemTxt := props.Text{Size: 8.5, Left: 1, Top: 1}
	itemRightTxt := props.Text{Size: 8.5, Align: align.Right, Right: 1, Top: 1}
	itemHeight := 7.0
	bodyHeight := 140.0
	for i, it := range inv.Items {
		unit := 0.0
		if it.Quantity > 0 {
			unit = it.Amount / float64(it.Quantity)
		}
		rowBorder := border.Left | border.Right
		if i == len(inv.Items)-1 && float64(len(inv.Items))*itemHeight >= bodyHeight {
			rowBorder |= border.Bottom
		}
		m.AddRow(itemHeight,
			cellCol(4, invoiceItemCode(it), itemTxt, rowBorder),
			cellCol(12, invoiceItemName(it), itemTxt, rowBorder),
			cellCol(2, fmt.Sprintf("%d", it.Quantity), itemRightTxt, rowBorder),
			cellCol(3, money(unit), itemRightTxt, rowBorder),
			cellCol(3, money(it.Amount), itemRightTxt, rowBorder),
		)
	}

	fillerHeight := bodyHeight - float64(len(inv.Items))*itemHeight
	if fillerHeight > 0 {
		m.AddRow(fillerHeight,
			cellCol(4, "", props.Text{}, border.Left|border.Right|border.Bottom),
			cellCol(12, "", props.Text{}, border.Left|border.Right|border.Bottom),
			cellCol(2, "", props.Text{}, border.Left|border.Right|border.Bottom),
			cellCol(3, "", props.Text{}, border.Left|border.Right|border.Bottom),
			cellCol(3, "", props.Text{}, border.Left|border.Right|border.Bottom),
		)
	}

	words := curLabel + " " + numberToWords(int64(inv.FinalAmount)) + " Only."

	m.AddRow(8,
		text.NewCol(17, words, props.Text{Size: 8.5, Top: 1}),
		cellCol(4, "Gross Total", props.Text{Size: 8.8, Style: fontstyle.Bold, Left: 1, Top: 1.3}, border.Left|border.Top|border.Bottom),
		cellCol(3, money(inv.Amount), props.Text{Size: 8.8, Align: align.Right, Right: 1, Top: 1.3}, border.Right|border.Top|border.Bottom),
	)
	if inv.DiscountValue > 0 {
		m.AddRow(7,
			text.NewCol(17, "", props.Text{}),
			cellCol(4, "Discount", props.Text{Size: 8.8, Style: fontstyle.Bold, Left: 1, Top: 1}, border.Left),
			cellCol(3, "-"+money(inv.DiscountValue), props.Text{Size: 8.8, Align: align.Right, Right: 1, Top: 1}, border.Right),
		)
	}
	m.AddRow(8,
		text.NewCol(17, "", props.Text{}),
		cellCol(4, "Net", props.Text{Size: 9, Style: fontstyle.Bold, Left: 1, Top: 1.4}, border.Left|border.Top|border.Bottom),
		cellCol(3, money(inv.FinalAmount), props.Text{Size: 9, Style: fontstyle.Bold, Align: align.Right, Right: 1, Top: 1.4}, border.Right|border.Top|border.Bottom),
	)
	return save(m, "invoice")
}

type invoiceInfoRow struct {
	Label string
	Value string
}

func invoiceCurrency(currencyID string) string {
	return "USD"
}

func addInvoiceInfoRow(m core.Maroto, row, last int, left, right invoiceInfoRow, labelTxt, valueTxt props.Text) {
	leftLabelBorder := border.Left
	leftValueBorder := border.Right
	rightLabelBorder := border.Left
	rightValueBorder := border.Right
	if row == 0 {
		leftLabelBorder |= border.Top
		leftValueBorder |= border.Top
		rightLabelBorder |= border.Top
		rightValueBorder |= border.Top
	}
	if row == last {
		leftLabelBorder |= border.Bottom
		leftValueBorder |= border.Bottom
		rightLabelBorder |= border.Bottom
		rightValueBorder |= border.Bottom
	}

	leftValue := ""
	if left.Label != "" || left.Value != "" {
		leftValue = ": " + left.Value
	}
	rightValue := ""
	if right.Label != "" || right.Value != "" {
		rightValue = ": " + right.Value
	}

	m.AddRow(6,
		cellCol(4, left.Label, labelTxt, leftLabelBorder),
		cellCol(11, leftValue, valueTxt, leftValueBorder),
		text.NewCol(1, "", props.Text{}),
		cellCol(3, right.Label, labelTxt, rightLabelBorder),
		cellCol(5, rightValue, valueTxt, rightValueBorder),
	)
}

func cellCol(size int, value string, p props.Text, bt border.Type) core.Col {
	col := text.NewCol(size, value, p)
	if bt != border.None {
		col.WithStyle(&props.Cell{BorderType: bt, BorderThickness: 0.12})
	}
	return col
}

func patientDisplayName(patient store.Patient, fallback string) string {
	name := strings.Join(compactNonEmpty(patient.FirstName, patient.MiddleName, patient.LastName), " ")
	if name != "" {
		return name
	}
	return fallback
}

func invoicePartyCode(id string) string {
	compact := strings.ToUpper(strings.ReplaceAll(id, "-", ""))
	if len(compact) > 10 {
		return compact[:10]
	}
	return compact
}

func invoiceItemCode(it store.InvoiceItem) string {
	prefix := "X"
	switch it.ItemType {
	case "product":
		prefix = "M"
	case "procedure":
		prefix = "C"
	case "gift":
		prefix = "G"
	}
	compact := strings.ToUpper(strings.ReplaceAll(it.ItemID, "-", ""))
	if compact == "" {
		compact = strings.ToUpper(strings.ReplaceAll(it.ID, "-", ""))
	}
	if len(compact) > 6 {
		compact = compact[:6]
	}
	return prefix + compact
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

func save(m core.Maroto, name string) (string, error) {
	doc, err := m.Generate()
	if err != nil {
		return "", err
	}
	path := tmpPath(name)
	if err := doc.Save(path); err != nil {
		return "", err
	}
	return path, nil
}
