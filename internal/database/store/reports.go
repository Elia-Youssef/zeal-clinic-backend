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
type RevenueParams struct {
	From       string // YYYY-MM-DD inclusive
	To         string // YYYY-MM-DD inclusive
	ItemKind   string // "" | "products" | "procedures" | "all"
	TypeID     string // procedure type id
	CategoryID string // procedure or product category id
	CurrencyID string // optional
	// Level pins the grouping to a flat level across the whole dataset,
	// bypassing the tree drill-down. TypeID / CategoryID then act as
	// filters that scope which rows feed the aggregation.
	// "" | "kind" | "procedure-type" | "procedure-category" | "product-category" | "procedure" | "product"
	Level string
}

type RevenueGroup struct {
	GroupType  string  `json:"groupType"` // kind | procedure-type | procedure-category | product-category | procedure | product
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
	default:
		return kindGroups(p)
	}
}

// kindGroups: the top level (procedures vs products).
func kindGroups(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT ii.item_type,
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		WHERE tb.entity_type = 'patient'
		AND ii.item_type IN ('product','procedure')
		AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.From, p.To}
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
		if kind == "product" {
			g.EntityName = "Products"
		} else {
			g.EntityName = "Procedures"
		}
		out.Items = append(out.Items, g)
	}
	finalize(&out)
	return out, nil
}

// procedureTypeGroups: for ItemKind="procedures", group by procedure_types.
func procedureTypeGroups(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT COALESCE(pt.id, ''), COALESCE(NULLIF(pt.name, ''), 'Uncategorized'),
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances tb ON tb.id = i.to_balance_id
		JOIN procedures pr ON pr.id = ii.item_id
		LEFT JOIN procedure_types pt ON pt.id = pr.type_id
		WHERE tb.entity_type = 'patient'
		AND ii.item_type = 'procedure'
		AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.From, p.To}
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
		       COALESCE(SUM(ii.final_amount), 0)
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
		AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.From, p.To}
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
		       COALESCE(SUM(ii.final_amount), 0)
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
		AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.TypeID, p.From, p.To}
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
		SELECT pc.id, COALESCE(NULLIF(pc.name, ''), 'Unnamed'),
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM procedure_categories pc
		LEFT JOIN procedures pr ON pr.category_id = pc.id
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pc.parent_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.CategoryID, p.From, p.To}
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
		SELECT pc.id, COALESCE(NULLIF(pc.name, ''), 'Unnamed'),
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM product_categories pc
		LEFT JOIN products pd ON pd.category_id = pc.id
		LEFT JOIN invoice_items ii ON ii.item_id = pd.id AND ii.item_type = 'product'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pc.parent_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.CategoryID, p.From, p.To}
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
		SELECT pr.id, pr.name,
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM procedures pr
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pr.category_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.CategoryID, p.From, p.To}
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
		       COALESCE(SUM(ii.final_amount), 0)
		FROM products pd
		LEFT JOIN invoice_items ii ON ii.item_id = pd.id AND ii.item_type = 'product'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE pd.category_id = ?
		AND (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.CategoryID, p.From, p.To}
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
	default:
		return RevenueReport{}, fmt.Errorf("unsupported level %q", p.Level)
	}
}

func allProcedureTypes(p RevenueParams) (RevenueReport, error) {
	q := `
		SELECT pt.id, pt.name,
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM procedure_types pt
		LEFT JOIN procedures pr ON pr.type_id = pt.id
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.From, p.To}
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
		SELECT pc.id, pc.name,
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM procedure_categories pc
		` + prJoin + `
		LEFT JOIN invoice_items ii ON ii.item_id = pr.id AND ii.item_type = 'procedure'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := append(prArgs, p.From, p.To)
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
		SELECT pc.id, pc.name,
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM product_categories pc
		LEFT JOIN products pd ON pd.category_id = pc.id
		LEFT JOIN invoice_items ii ON ii.item_id = pd.id AND ii.item_type = 'product'
		LEFT JOIN invoices i ON i.id = ii.invoice_id
		LEFT JOIN balances tb ON tb.id = i.to_balance_id
		WHERE (ii.id IS NULL OR (
			tb.entity_type = 'patient'
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args := []any{p.From, p.To}
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
		SELECT pr.id, pr.name,
		       COALESCE(SUM(ii.quantity), 0),
		       COALESCE(SUM(ii.final_amount), 0)
		FROM procedures pr
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
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args = append(args, p.From, p.To)
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
		       COALESCE(SUM(ii.final_amount), 0)
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
			AND date(i.created_at) BETWEEN date(?) AND date(?)`
	args = append(args, p.From, p.To)
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
	Date             Date    `json:"date"`
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

// Expenses returns one row per supplier-invoice line item plus one row per
// expense-entity payment, sorted by date asc. Remaining balance reflects the
// supplier's current outstanding balance (snapshot, not historical).
func (Reports) Expenses(p ExpensesParams) ([]ExpenseRow, error) {
	if p.From == "" || p.To == "" {
		return nil, fmt.Errorf("from and to are required")
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
		       i.notes
		FROM invoice_items ii
		JOIN invoices i ON i.id = ii.invoice_id
		JOIN balances fb ON fb.id = i.from_balance_id
		LEFT JOIN products pd  ON pd.id = ii.item_id AND ii.item_type = 'product'
		LEFT JOIN procedures pr ON pr.id = ii.item_id AND ii.item_type = 'procedure'
		WHERE fb.entity_type = 'supplier'
		AND date(i.created_at) BETWEEN date(?) AND date(?)`
	itemArgs := []any{p.From, p.To}
	if p.CurrencyID != "" {
		itemQ += " AND i.currency_id = ?"
		itemArgs = append(itemArgs, p.CurrencyID)
	}

	rows, err := RDB.Query(itemQ, itemArgs...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r ExpenseRow
		if err := rows.Scan(&r.Date, &r.Supplier, &r.Description, &r.Quantity, &r.Amount, &r.RemainingBalance, &r.Notes); err != nil {
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
		return nil, err
	}

	// Expense-entity payments: direct outflows (CreateTwoWay nets to 0).
	payQ := `
		SELECT bt.created_at, tb.entity_name, bt.description, bt.amount, COALESCE(tb.amount, 0)
		FROM balance_transactions bt
		JOIN balances fb ON fb.id = bt.from_balance_id
		JOIN balances tb ON tb.id = bt.to_balance_id
		WHERE fb.entity_type = 'self'
		AND tb.entity_type = 'expense'
		AND bt.voided_at = ''
		AND bt.transaction_type = 'payment'
		AND date(bt.created_at) BETWEEN date(?) AND date(?)`
	payArgs := []any{p.From, p.To}
	if p.CurrencyID != "" {
		payQ += " AND bt.currency_id = ?"
		payArgs = append(payArgs, p.CurrencyID)
	}

	prows, err := RDB.Query(payQ, payArgs...)
	if err != nil {
		return nil, err
	}
	for prows.Next() {
		var r ExpenseRow
		if err := prows.Scan(&r.Date, &r.Supplier, &r.Description, &r.Amount, &r.RemainingBalance); err != nil {
			continue
		}
		r.Quantity = 1
		r.AmountPerUnit = r.Amount
		r.Total = r.Amount + r.RemainingBalance
		out = append(out, r)
	}
	prows.Close()
	if err := prows.Err(); err != nil {
		return nil, err
	}

	// Sort by date asc: simple insertion sort, list is small.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Date.Before(out[j-1].Date); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}
