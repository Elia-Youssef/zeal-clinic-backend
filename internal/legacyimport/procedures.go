package legacyimport

import (
	"sort"
	"strings"

	"clinic-api/internal/database/store"
	"clinic-api/internal/legacyimport/conv"
	"clinic-api/internal/legacyimport/csvutil"
)

func legacyProcedureType(sheet string) string {
	switch strings.ToLower(strings.TrimSpace(sheet)) {
	case "clinic procedure":
		return "Clinic Procedure"
	case "hospital surgery":
		return "Hospital Surgery"
	case "minor surgery":
		return "Minor Surgery"
	default:
		return ""
	}
}

func migrateProcedures(c *Context, codeLabels map[string]string) (procs, prices *rowset, err error) {
	f, err := c.File("procedures.csv")
	if err != nil {
		return nil, nil, err
	}
	sec := c.Report.Section("procedures (deprecated)")
	procs = newRowset("id", "name", "type_id", "category_id", "is_active", "remarks", "created_at")
	prices = newRowset("id", "procedure_id", "price", "is_active")

	col := func(n string) int { return f.Col(n) }
	var read, created, dupName, badPrice int
	seenName := map[string]bool{}

	for _, row := range f.Rows {
		id := csvutil.Get(row, col("id"))
		name := csvutil.Get(row, col("name"))
		if id == "" || id == "0" || name == "" {
			continue
		}
		if strings.EqualFold(csvutil.Get(row, col("sheet")), "STOCK") {
			continue
		}
		read++
		if seenName[name] {
			dupName++
			continue
		}
		seenName[name] = true

		typeID := c.procTypeID[legacyProcedureType(csvutil.Get(row, col("sheet")))]
		remarks := ""
		if cat := csvutil.Get(row, col("procedure")); cat != "" {
			remarks = "Legacy category: " + cat
		}
		price, okp := conv.Float(csvutil.Get(row, col("stand_pr")), 0)
		if !okp {
			badPrice++
		}

		procs.add(id, name+" (deprecated)", typeID, "", 0, remarks, c.MigrationTS)
		prices.add(c.ID("", "procedure_price:"+id), id, store.Round2(price), 1)

		c.procByName[name] = id
		created++
	}

	categories := 0
	for _, name := range sortedValues(codeLabels) {
		if c.procByCategory[name] != "" {
			continue
		}
		if existing := c.procByName[name]; existing != "" {
			c.procByCategory[name] = existing
			continue
		}
		id := c.ID("", "procedure:category:"+name)
		procs.add(id, name+" (deprecated)", "", "", 0, "Legacy appointment category", c.MigrationTS)
		c.procByCategory[name] = id
		categories++
	}

	sec.Counts(read, created+categories, dupName)
	sec.Note("Legacy procedure catalog imported as deprecated procedures (name tagged ' (deprecated)', is_active=0). The seeded catalog is untouched.")
	sec.Note("Legacy category preserved in remarks; type mapped from the source sheet; stand_pr -> active price.")
	sec.Note("Added %d category-level deprecated procedure(s) so appointments (which reference a category, not a specific procedure) can be linked.", categories)
	if dupName > 0 {
		sec.Note("%d repeated procedure name(s) collapsed to one row each.", dupName)
	}
	if badPrice > 0 {
		sec.Issue(c.Report, "%d procedure(s) had a non-numeric price (defaulted to 0).", badPrice)
	}
	return procs, prices, nil
}

func sortedValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	seen := map[string]bool{}
	for _, v := range m {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
