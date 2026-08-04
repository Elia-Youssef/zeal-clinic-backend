package pdf

import (
	"clinic-api/internal/assets"

	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// Shared, print-economical styling so invoices, reports and appointment sheets
// share the analytics PDF's visual language (masthead, accent section bars,
// clean grid tables) without the ink cost of dark header fills or zebra rows.
// Tables use light tile headers, white body rows, and hairline grids; colors
// come from the shared brand palette in analytics.go.

// masthead renders the logo, "Zeal Clinic" + a document subtitle, and a
// right-aligned meta block, closed by a thin rule. grid is the document's
// MaxGridSize so column widths scale to portrait (24) or landscape (100).
func masthead(s rowSink, grid int, subtitle string, metaLines []string) {
	logoW := grid * 18 / 100
	if logoW < 3 {
		logoW = 3
	}
	metaW := grid * 34 / 100
	if metaW < 6 {
		metaW = 6
	}
	brandW := grid - logoW - metaW

	brand := []core.Component{
		text.New("Zeal Clinic", props.Text{Size: 15, Style: fontstyle.Bold, Color: clrInk, Top: 2}),
		text.New(subtitle, props.Text{Size: 9, Color: clrMutedFg, Top: 8.5}),
	}

	meta := make([]core.Component, 0, len(metaLines))
	top := 2.5
	for i, line := range metaLines {
		p := props.Text{Size: 8, Color: clrMutedFg, Align: align.Right, Right: 1, Top: top}
		if i == 0 {
			p.Size = 9.5
			p.Style = fontstyle.Bold
			p.Color = clrInk
		}
		meta = append(meta, text.New(line, p))
		top += 5
	}

	var cols []core.Col
	if logo := assets.InvoiceLogo(); len(logo) > 0 {
		cols = append(cols, image.NewFromBytesCol(logoW, logo, extension.Png, props.Rect{Percent: 92, Center: true}))
	} else {
		brandW += logoW
	}
	cols = append(cols, col.New(brandW).Add(brand...), col.New(metaW).Add(meta...))

	s.AddRow(15, cols...)
	s.AddRow(2, col.New(grid).WithStyle(&props.Cell{BorderType: border.Bottom, BorderColor: clrPrimary, BorderThickness: 0.4}))
	s.AddRow(2)
}

// titleBar is the accent-tab + light-tile section header from the analytics PDF.
func titleBar(s rowSink, grid int, title string) {
	accent := grid/50 + 1
	s.AddRow(4)
	s.AddRow(8,
		col.New(accent).WithStyle(&props.Cell{BackgroundColor: clrAccent}),
		text.NewCol(grid-accent, title, props.Text{Size: 10, Style: fontstyle.Bold, Color: clrPrimary, Left: 2.5, Top: 2}).
			WithStyle(&props.Cell{BackgroundColor: clrTile}),
	)
	s.AddRow(1.5)
}

// headerCell: light tile fill, bold ink, for the table column header.
func headerCell(size int, s string, a align.Type) core.Col {
	c := text.NewCol(size, s, props.Text{Size: 8.4, Style: fontstyle.Bold, Color: clrInk, Align: a, Left: padLeft(a), Right: padRight(a), Top: 1.3})
	c.WithStyle(&props.Cell{BackgroundColor: clrTile, BorderType: border.Full, BorderColor: clrBorder, BorderThickness: 0.1})
	return c
}

// bodyCell: white background, hairline grid border.
// bodyCell's Bottom padding is read only by AddAutoRow (maroto ignores it on
// fixed-height rows), where it keeps a single-line row at the same ~7mm the
// fixed tables use: 1.1 top + 2.9 of 8.2pt text + 3.0 bottom.
func bodyCell(size int, s string, a align.Type) core.Col {
	return bodyCellColor(size, s, a, clrInk)
}

// bodyCellColor is bodyCell with the text colour overridden, used to flag a
// row's state (a cancelled appointment) without disturbing the grid.
func bodyCellColor(size int, s string, a align.Type, color *props.Color) core.Col {
	c := text.NewCol(size, s, props.Text{Size: 8.2, Color: color, Align: a, Left: padLeft(a), Right: padRight(a), Top: 1.1, Bottom: 3.0})
	c.WithStyle(&props.Cell{BorderType: border.Full, BorderColor: clrBorder, BorderThickness: 0.1})
	return c
}

// boldCell: white background, bold text for emphasized body rows (e.g. subtotal labels).
func boldCell(size int, s string, a align.Type) core.Col {
	c := text.NewCol(size, s, props.Text{Size: 8.6, Style: fontstyle.Bold, Color: clrInk, Align: a, Left: padLeft(a), Right: padRight(a), Top: 1.2})
	c.WithStyle(&props.Cell{BorderType: border.Full, BorderColor: clrBorder, BorderThickness: 0.1})
	return c
}

// emphCell: light tile fill + bold, for totals / grand-total rows.
func emphCell(size int, s string, a align.Type) core.Col {
	c := text.NewCol(size, s, props.Text{Size: 8.8, Style: fontstyle.Bold, Color: clrInk, Align: a, Left: padLeft(a), Right: padRight(a), Top: 1.3})
	c.WithStyle(&props.Cell{BackgroundColor: clrTile, BorderType: border.Full, BorderColor: clrBorder, BorderThickness: 0.1})
	return c
}
