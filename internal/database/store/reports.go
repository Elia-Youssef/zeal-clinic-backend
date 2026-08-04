package store

import (
	"database/sql"
	"fmt"
)

type Reports struct{}

// Revenue report

// RevenueParams scopes a revenue report to a date range and a position in the
// product / procedure tree. Position is described by ItemKind (top toggle),
// TypeID (procedure-type drill), and CategoryID (category drill; works for
// both procedure and product categories). Each level shows aggregates for the
// direct children of the current node.
// Date range bounds: accept either RFC3339 UTC instants (`from` inclusive,
// `to` exclusive; the frontend converts clinic-local boundaries to UTC) or bare
// YYYY-MM-DD UTC dates (`from` becomes start-of-day, `to` becomes start of
// the following day so the calendar day is included).
type RevenueParams struct {
	From       string
	To         string
	ItemKind   string // "" | "products" | "procedures" | "other" | "discounts" | "all"
	TypeID     string // procedure type id
	CategoryID string // procedure or product category id
	CurrencyID string
	// Level pins the grouping to a flat level across the whole dataset,
	// bypassing the tree drill-down. TypeID / CategoryID then act as
	// filters that scope which rows feed the aggregation.
	// "" | "kind" | "procedure-type" | "procedure-category" | "product-category" | "procedure" | "product" | "other" | "discount" | "all"
	Level string
}

type RevenueGroup struct {
	GroupType  string  `json:"groupType"` // kind | procedure-type | procedure-category | product-category | procedure | product | other | discount
	EntityID   string  `json:"entityId"`
	EntityName string  `json:"entityName"`
	Quantity   int     `json:"quantity"`
	Amount     float64 `json:"amount"`
	Percentage float64 `json:"percentage"`
}

type RevenueTotals struct {
	Quantity int     `json:"quantity"`
	Amount   float64 `json:"amount"`
}

type RevenueReport struct {
	Items  []RevenueGroup `json:"items"`
	Totals RevenueTotals  `json:"totals"`
	Level  string         `json:"level"` // describes the grouping for the client
}

// Revenue aggregates revenue (client invoices) by the immediate children of
// the requested tree position, returning per-group quantity/amount and the
// percentage each group represents of the total.
func (Reports) Revenue(p RevenueParams) (RevenueReport, error) {
	if p.From == "" || p.To == "" {
		return RevenueReport{}, fmt.Errorf("from and to are required")
	}
	if p.CurrencyID == "" {
		p.CurrencyID = USDCurrencyID
	}

	// Flat-level mode bypasses tree drill-down: caller asked for "all X".
	if p.Level != "" {
		return flatLevel(p)
	}

	// Drill into a category (either procedure or product). If the category has
	// child categories, group by those; otherwise group by the procedures or
	// products that belong to it (the leaves).
	if p.CategoryID != "" {
		return categoryDrill(p)
	}

	// Drill into a procedure type: group by the top-level procedure categories
	// containing procedures of that type.
	if p.TypeID != "" {
		return procedureCategoriesByType(p)
	}

	switch p.ItemKind {
	case "procedures":
		return procedureTypeGroups(p)
	case "products":
		return topProductCategoryGroups(p)
	case "other":
		return otherGroups(p)
	case "discounts":
		return discountGroups(p)
	case "all":
		return allItems(p)
	default:
		return kindGroups(p)
	}
}

// kindGroups: the top level (procedures, products and other lines). Gifts are
// excluded (gift-card sales are deferred revenue, counted on redemption).
func kindGroups(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT ii.item_type,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'
		AND ii.item_type IN ('product','procedure','other')
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{RangeStart(p.From), RangeEnd(p.To)}
	q, args = applyCurrency(q, args, "i.currency_id", p.CurrencyID)
	q += " GROUP BY ii.item_type"

	rows, err := RDB.Query(q, args...)
	if err != nil {
		return RevenueReport{}, err
	}
	defer rows.Close()

	out := RevenueReport{Level: "kind"}
	for rows.Next() {
		var g RevenueGroup
		var kind string
		if err := rows.Scan(&kind, &g.Quantity, &g.Amount); err != nil {
			continue
		}
		g.GroupType = "kind"
		g.EntityID = kind
		g.EntityName = kindDisplayName(kind)
		out.Items = append(out.Items, g)
	}
	finalize(&out)
	return out, nil
}

// kindDisplayName maps an invoice_items.item_type to its report label.
func kindDisplayName(kind string) string {
	switch kind {
	case "product":
		return "Products"
	case "procedure":
		return "Procedures"
	default:
		return "Other"
	}
}

// otherGroups: for ItemKind="other", group miscellaneous lines by their note
// (the only label "other" lines carry; they have no catalog item).
func otherGroups(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT '', COALESCE(NULLIF(ii.notes, ''), 'Other'),
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'
		AND ii.item_type = 'other'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{RangeStart(p.From), RangeEnd(p.To)}
	q, args = applyCurrency(q, args, "i.currency_id", p.CurrencyID)
	q += `
		GROUP BY ii.notes
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "other")
	if err != nil {
		return out, err
	}
	out.Level = "other"
	return out, nil
}

// discountGroups: for ItemKind="discounts" / Level="discount", invoice offers,
// grouped by the discount applied, with quantity counting the invoices it was
// applied to. Amounts are negative: this is revenue the clinic gave up, not
// revenue it earned, which is also why the kind is deliberately absent from
// kindGroups and from "all": mixing it in would corrupt those totals.
// Gift cards are not included; a card is a balance credit the patient spends
// like cash, not a discount on an invoice.
func discountGroups(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT d.id, d.name, COUNT(*),
		       COALESCE(SUM(i.discount_value), 0)
		FROM invoices i
		JOIN discounts d ON d.id = i.discount_id
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'
		AND i.discount_id != ''
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{RangeStart(p.From), RangeEnd(p.To)}
	q, args = applyCurrency(q, args, "i.currency_id", p.CurrencyID)
	q += `
		GROUP BY d.id
		ORDER BY 4 DESC`

	// Aggregated as positive magnitudes so scanGroups' ranking and percentage
	// share come out the usual way (biggest discount first), then flipped.
	out, err := scanGroups(q, args, "discount")
	if err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i].Amount = -out.Items[i].Amount
	}
	out.Totals.Amount = -out.Totals.Amount
	out.Level = "discount"
	return out, nil
}

// allItems: for ItemKind/Level="all", every individual procedure, product and
// other line across the dataset, ranked together. Composes the per-kind leaf
// aggregators and re-finalizes so totals and percentages span all three.
func allItems(p RevenueParams) (RevenueReport, error) {
	procs, err := allProcedures(p)
	if err != nil {
		return RevenueReport{}, err
	}
	prods, err := allProducts(p)
	if err != nil {
		return RevenueReport{}, err
	}
	others, err := otherGroups(p)
	if err != nil {
		return RevenueReport{}, err
	}

	out := RevenueReport{Level: "all"}
	out.Items = append(out.Items, procs.Items...)
	out.Items = append(out.Items, prods.Items...)
	out.Items = append(out.Items, others.Items...)
	finalize(&out)
	return out, nil
}

// procedureTypeGroups: for ItemKind="procedures", group by procedure_types.
func procedureTypeGroups(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT COALESCE(pt.id, ''), COALESCE(NULLIF(pt.name, ''), 'Uncategorized'),
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		JOIN procedures pr ON pr.id = ii.item_id
		LEFT JOIN procedure_types pt ON pt.id = pr.type_id
		WHERE tb.entity_type = 'patient'
		AND ii.item_type = 'procedure'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{RangeStart(p.From), RangeEnd(p.To)}
	q, args = applyCurrency(q, args, "i.currency_id", p.CurrencyID)
	q += `
		GROUP BY pt.id
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "procedure-type")
	if err != nil {
		return out, err
	}
	out.Level = "procedure-type"
	return out, nil
}

// topProductCategoryGroups: for ItemKind="products", group by top-level product
// categories. Products whose category has a parent roll up to the parent.
func topProductCategoryGroups(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT COALESCE(top.id, ''), COALESCE(NULLIF(top.name, ''), 'Uncategorized'),
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		JOIN products pd ON pd.id = ii.item_id
		LEFT JOIN product_categories self ON self.id = pd.category_id
		LEFT JOIN product_categories top
			ON top.id = CASE WHEN self.parent_id = '' OR self.parent_id IS NULL
			                 THEN self.id ELSE self.parent_id END
		WHERE tb.entity_type = 'patient'
		AND ii.item_type = 'product'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{RangeStart(p.From), RangeEnd(p.To)}
	q, args = applyCurrency(q, args, "i.currency_id", p.CurrencyID)
	q += `
		GROUP BY top.id
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "product-category")
	if err != nil {
		return out, err
	}
	out.Level = "product-category"
	return out, nil
}

// procedureCategoriesByType: with TypeID set, group by top-level procedure
// categories that contain procedures of this type. Procedures whose category
// has a parent roll up to the parent.
func procedureCategoriesByType(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT COALESCE(top.id, ''), COALESCE(NULLIF(top.name, ''), 'Uncategorized'),
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		JOIN procedures pr ON pr.id = ii.item_id
		LEFT JOIN procedure_categories self ON self.id = pr.category_id
		LEFT JOIN procedure_categories top
			ON top.id = CASE WHEN self.parent_id = '' OR self.parent_id IS NULL
			                 THEN self.id ELSE self.parent_id END
		WHERE tb.entity_type = 'patient'
		AND ii.item_type = 'procedure'
		AND pr.type_id = ?
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{p.TypeID, RangeStart(p.From), RangeEnd(p.To)}
	q, args = applyCurrency(q, args, "i.currency_id", p.CurrencyID)
	q += `
		GROUP BY top.id
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "procedure-category")
	if err != nil {
		return out, err
	}
	out.Level = "procedure-category"
	return out, nil
}

// categoryDrill: with CategoryID set, figure out which kind of category it is,
// then group by direct child categories (if any) or by individual leaf items.
func categoryDrill(p RevenueParams) (RevenueReport, error) {
	kind, err := categoryKind(p.CategoryID)
	if err != nil {
		return RevenueReport{}, err
	}
	if kind == "" {
		return RevenueReport{}, fmt.Errorf("category not found")
	}

	hasChildren, err := categoryHasChildren(p.CategoryID, kind)
	if err != nil {
		return RevenueReport{}, err
	}

	if kind == "procedure" {
		if hasChildren {
			return procedureChildCategories(p)
		}
		return procedureLeaves(p)
	}
	if hasChildren {
		return productChildCategories(p)
	}
	return productLeaves(p)
}

func procedureChildCategories(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pc.id, ` + categoryNameWithParentExpr() + `,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM procedure_categories pc
		LEFT JOIN procedure_categories pcparent ON pcparent.id = pc.parent_id
		LEFT JOIN procedures pr ON pr.category_id = pc.id
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pc.parent_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{p.CategoryID, RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pc.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "procedure-category")
	if err != nil {
		return out, err
	}
	out.Level = "procedure-category"
	return out, nil
}

func productChildCategories(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pc.id, ` + categoryNameWithParentExpr() + `,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM product_categories pc
		LEFT JOIN product_categories pcparent ON pcparent.id = pc.parent_id
		LEFT JOIN products pd ON pd.category_id = pc.id
		LEFT JOIN invoice_items ii ON ii.item_id = pd.id AND ii.item_type = 'product'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pc.parent_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{p.CategoryID, RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pc.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "product-category")
	if err != nil {
		return out, err
	}
	out.Level = "product-category"
	return out, nil
}

func procedureLeaves(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pr.id, ` + procedureNameWithCategoryExpr() + `,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM procedures pr
		LEFT JOIN procedure_categories pcat ON pcat.id = pr.category_id
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pr.category_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{p.CategoryID, RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pr.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "procedure")
	if err != nil {
		return out, err
	}
	out.Level = "procedure"
	return out, nil
}

func productLeaves(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pd.id, pd.name,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM products pd
		LEFT JOIN invoice_items ii ON ii.item_id = pd.id AND ii.item_type = 'product'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pd.category_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{p.CategoryID, RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pd.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "product")
	if err != nil {
		return out, err
	}
	out.Level = "product"
	return out, nil
}

// flat-level mode

// flatLevel dispatches to the per-level aggregator. TypeID / CategoryID still
// act as scope filters on the underlying data when applicable.
func flatLevel(p RevenueParams) (RevenueReport, error) {
	switch p.Level {
	case "kind":
		return kindGroups(p)
	case "procedure-type":
		return allProcedureTypes(p)
	case "procedure-category":
		return allProcedureCategories(p)
	case "product-category":
		return allProductCategories(p)
	case "procedure":
		return allProcedures(p)
	case "product":
		return allProducts(p)
	case "other":
		return otherGroups(p)
	case "discount":
		return discountGroups(p)
	case "all":
		return allItems(p)
	default:
		return RevenueReport{}, fmt.Errorf("unsupported level %q", p.Level)
	}
}

func allProcedureTypes(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pt.id, pt.name,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM procedure_types pt
		LEFT JOIN procedures pr ON pr.type_id = pt.id
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pt.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "procedure-type")
	if err != nil {
		return out, err
	}
	out.Level = "procedure-type"
	return out, nil
}

func allProcedureCategories(p RevenueParams) (RevenueReport, error) {
	prJoin := `LEFT JOIN procedures pr ON pr.category_id = pc.id`
	prArgs := []any{}
	if p.TypeID != "" {
		prJoin = `LEFT JOIN procedures pr ON pr.category_id = pc.id AND pr.type_id = ?`
		prArgs = append(prArgs, p.TypeID)
	}
	q := `
		SELECT pc.id, ` + categoryNameWithParentExpr() + `,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM procedure_categories pc
		LEFT JOIN procedure_categories pcparent ON pcparent.id = pc.parent_id
		` + prJoin + `
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := append(prArgs, RangeStart(p.From), RangeEnd(p.To))
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pc.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "procedure-category")
	if err != nil {
		return out, err
	}
	out.Level = "procedure-category"
	return out, nil
}

func allProductCategories(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pc.id, ` + categoryNameWithParentExpr() + `,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM product_categories pc
		LEFT JOIN product_categories pcparent ON pcparent.id = pc.parent_id
		LEFT JOIN products pd ON pd.category_id = pc.id
		LEFT JOIN invoice_items ii ON ii.item_id = pd.id AND ii.item_type = 'product'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args := []any{RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pc.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "product-category")
	if err != nil {
		return out, err
	}
	out.Level = "product-category"
	return out, nil
}

func allProcedures(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pr.id, ` + procedureNameWithCategoryExpr() + `,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM procedures pr
		LEFT JOIN procedure_categories pcat ON pcat.id = pr.category_id
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE 1=1`
	args := []any{}
	if p.TypeID != "" {
		q += ` AND pr.type_id = ?`
		args = append(args, p.TypeID)
	}
	if p.CategoryID != "" {
		q += ` AND pr.category_id = ?`
		args = append(args, p.CategoryID)
	}
	q += ` AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args = append(args, RangeStart(p.From), RangeEnd(p.To))
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pr.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "procedure")
	if err != nil {
		return out, err
	}
	out.Level = "procedure"
	return out, nil
}

func allProducts(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pd.id, pd.name,
		       COALESCE(SUM(ii.quantity), 0),
		       ` + revenueAmountExpr() + `
		FROM products pd
		LEFT JOIN invoice_items ii ON ii.item_id = pd.id AND ii.item_type = 'product'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE 1=1`
	args := []any{}
	if p.CategoryID != "" {
		q += ` AND pd.category_id = ?`
		args = append(args, p.CategoryID)
	}
	q += ` AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	args = append(args, RangeStart(p.From), RangeEnd(p.To))
	if p.CurrencyID != "" {
		q += ` AND i.currency_id = ?`
		args = append(args, p.CurrencyID)
	}
	q += `))
		GROUP BY pd.id
		HAVING SUM(ii.quantity) > 0 OR SUM(ii.final_amount) > 0
		ORDER BY 4 DESC`

	out, err := scanGroups(q, args, "product")
	if err != nil {
		return out, err
	}
	out.Level = "product"
	return out, nil
}

// helpers

func categoryKind(id string) (string, error) {
	var n int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM procedure_categories WHERE id = ?`, id).Scan(&n); err != nil {
		return "", err
	}
	if n > 0 {
		return "procedure", nil
	}
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM product_categories WHERE id = ?`, id).Scan(&n); err != nil {
		return "", err
	}
	if n > 0 {
		return "product", nil
	}
	return "", nil
}

func categoryHasChildren(id, kind string) (bool, error) {
	var table string
	if kind == "procedure" {
		table = "procedure_categories"
	} else {
		table = "product_categories"
	}
	var n int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE parent_id = ?`, id).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// procedureNameWithCategoryExpr returns a SQL expression that prefixes the
// procedure name with its direct category (when set), matching the appointment
// store's "category, name" decoration. Requires the query to alias
// procedure_categories as pcat.
func procedureNameWithCategoryExpr() string {
	return `CASE
		WHEN COALESCE(pcat.name, '') != '' THEN pcat.name || ', ' || pr.name
		ELSE pr.name
	END`
}

// categoryNameWithParentExpr returns a SQL expression that prefixes a category
// name with its parent category (when set), separated by a comma. Requires
// the query to alias the displayed category as pc and the parent category as
// pcparent.
func categoryNameWithParentExpr() string {
	return `CASE
		WHEN COALESCE(pcparent.name, '') != '' THEN pcparent.name || ', ' || COALESCE(NULLIF(pc.name, ''), 'Unnamed')
		ELSE COALESCE(NULLIF(pc.name, ''), 'Unnamed')
	END`
}

// revenueAmountExpr: item final_amount minus its share of the invoice offer
// discount. Requires aliases invoice_items ii and invoices i.
func revenueAmountExpr() string {
	return `COALESCE(SUM(ii.final_amount - i.discount_value * ii.final_amount / NULLIF(i.amount, 0)), 0)`
}

func applyCurrency(q string, args []any, col, currencyID string) (string, []any) {
	if currencyID == "" {
		return q, args
	}
	return q + " AND " + col + " = ?", append(args, currencyID)
}

func scanGroups(query string, args []any, groupType string) (RevenueReport, error) {
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return RevenueReport{}, err
	}
	defer rows.Close()

	out := RevenueReport{}
	for rows.Next() {
		var g RevenueGroup
		var qty sql.NullInt64
		var amt sql.NullFloat64
		if err := rows.Scan(&g.EntityID, &g.EntityName, &qty, &amt); err != nil {
			continue
		}
		g.Quantity = int(qty.Int64)
		g.Amount = amt.Float64
		g.GroupType = groupType
		out.Items = append(out.Items, g)
	}
	finalize(&out)
	return out, nil
}

// finalize computes totals and percentages for the items, sorted by amount desc.
func finalize(r *RevenueReport) {
	if r.Items == nil {
		r.Items = []RevenueGroup{}
	}
	for _, g := range r.Items {
		r.Totals.Quantity += g.Quantity
		r.Totals.Amount += g.Amount
	}
	if r.Totals.Amount > 0 {
		for i := range r.Items {
			r.Items[i].Percentage = r.Items[i].Amount / r.Totals.Amount * 100
		}
	}
	// Sort by amount desc. Most queries already ORDER BY amount, but the
	// hierarchical drills include zero-revenue children via LEFT JOIN, so
	// re-sort here for consistency.
	for i := 1; i < len(r.Items); i++ {
		for j := i; j > 0 && r.Items[j].Amount > r.Items[j-1].Amount; j-- {
			r.Items[j], r.Items[j-1] = r.Items[j-1], r.Items[j]
		}
	}
}

// Expenses report

type ExpenseRow struct {
	Date Date `json:"date"`
	// BalanceID is the supplier/expense entity's balance row id. Not serialized;
	// used to dedupe the remaining-balance total, which is a per-entity snapshot
	// repeated on every row that entity owns.
	BalanceID        string  `json:"-"`
	Supplier         string  `json:"supplier"`
	Description      string  `json:"description"`
	Quantity         int     `json:"quantity"`
	AmountPerUnit    float64 `json:"amountPerUnit"`
	Amount           float64 `json:"amount"`
	RemainingBalance float64 `json:"remainingBalance"`
	Total            float64 `json:"total"`
	Notes            string  `json:"notes"`
}

type ExpensesParams struct {
	From       string
	To         string
	CurrencyID string
}

type ExpensesTotals struct {
	Amount    float64 `json:"amount"`
	Remaining float64 `json:"remaining"`
}

type ExpensesReport struct {
	Rows   []ExpenseRow   `json:"rows"`
	Totals ExpensesTotals `json:"totals"`
}

// Expenses returns one row per supplier-invoice line item plus one row per
// expense-entity payment, sorted by date asc, with grand totals. Remaining
// balance reflects the supplier's current outstanding balance (snapshot, not
// historical); the remaining total counts each entity's balance once.
func (Reports) Expenses(p ExpensesParams) (ExpensesReport, error) {
	if p.From == "" || p.To == "" {
		return ExpensesReport{}, fmt.Errorf("from and to are required")
	}
	if p.CurrencyID == "" {
		p.CurrencyID = USDCurrencyID
	}

	out := []ExpenseRow{}

	// Supplier invoice items.
	itemQ := `
		SELECT i.created_at,
		       fb.entity_name,
		       CASE ii.item_type
		           WHEN 'product'   THEN COALESCE(pd.name, '')
		           WHEN 'procedure' THEN COALESCE(pr.name, '')
		           ELSE COALESCE(ii.notes, '')
		       END AS description,
		       ii.quantity,
		       ii.final_amount,
		       COALESCE(fb.amount, 0) AS remaining,
		       i.notes,
		       fb.id
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances fb ON fb.id = i.from_balance_id
		LEFT JOIN products pd  ON pd.id = ii.item_id AND ii.item_type = 'product'
		LEFT JOIN procedures pr ON pr.id = ii.item_id AND ii.item_type = 'procedure'
		WHERE fb.entity_type = 'supplier'
		AND i.created_at >= ? AND i.created_at < ?
			AND i.voided_at = ''`
	itemArgs := []any{RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		itemQ += " AND i.currency_id = ?"
		itemArgs = append(itemArgs, p.CurrencyID)
	}

	rows, err := RDB.Query(itemQ, itemArgs...)
	if err != nil {
		return ExpensesReport{}, err
	}
	for rows.Next() {
		var r ExpenseRow
		if err := rows.Scan(&r.Date, &r.Supplier, &r.Description, &r.Quantity, &r.Amount, &r.RemainingBalance, &r.Notes, &r.BalanceID); err != nil {
			continue
		}
		if r.Quantity > 0 {
			r.AmountPerUnit = r.Amount / float64(r.Quantity)
		} else {
			r.AmountPerUnit = r.Amount
		}
		r.Total = r.Amount + r.RemainingBalance
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ExpensesReport{}, err
	}

	// Expense-entity payments: direct outflows (CreateTwoWay nets to 0).
	payQ := `
		SELECT bt.created_at, tb.entity_name, bt.description, bt.amount, COALESCE(tb.amount, 0), tb.id
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE fb.entity_type = 'self'
		AND tb.entity_type = 'expense'
		AND bt.voided_at = ''
		AND bt.transaction_type = 'payment'
		AND bt.created_at >= ? AND bt.created_at < ?`
	payArgs := []any{RangeStart(p.From), RangeEnd(p.To)}
	if p.CurrencyID != "" {
		payQ += " AND bt.currency_id = ?"
		payArgs = append(payArgs, p.CurrencyID)
	}

	prows, err := RDB.Query(payQ, payArgs...)
	if err != nil {
		return ExpensesReport{}, err
	}
	for prows.Next() {
		var r ExpenseRow
		if err := prows.Scan(&r.Date, &r.Supplier, &r.Description, &r.Amount, &r.RemainingBalance, &r.BalanceID); err != nil {
			continue
		}
		r.Quantity = 1
		r.AmountPerUnit = r.Amount
		r.Total = r.Amount + r.RemainingBalance
		out = append(out, r)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return ExpensesReport{}, err
	}

	// Sort by date asc: simple insertion sort, list is small.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Date.Before(out[j-1].Date); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	// Totals: amount sums every row; remaining counts each entity's balance
	// snapshot once (it repeats across that entity's rows).
	report := ExpensesReport{Rows: out}
	countedRemaining := map[string]bool{}
	for _, r := range out {
		report.Totals.Amount += r.Amount
		if !countedRemaining[r.BalanceID] {
			countedRemaining[r.BalanceID] = true
			report.Totals.Remaining += r.RemainingBalance
		}
	}
	return report, nil
}
