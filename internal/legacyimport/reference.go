package legacyimport

import (
	"sort"

	"clinic-api/internal/database/store"
	"clinic-api/internal/legacyimport/conv"
	"clinic-api/internal/legacyimport/csvutil"
)

func scanExtraRooms(c *Context) (*rowset, error) {
	f, err := c.File("appointments.csv")
	if err != nil {
		return nil, err
	}
	sec := c.Report.Section("rooms (top-up)")
	t := newRowset("id", "name", "type", "is_available", "created_at")

	roomCol := f.Col("room")
	seen := map[string]bool{}
	needFallback := false
	var extra []string
	for _, row := range f.Rows {
		name := csvutil.Get(row, roomCol)
		if name == "" {
			needFallback = true
			continue
		}
		if seen[name] || c.roomID[name] != "" {
			continue
		}
		seen[name] = true
		extra = append(extra, name)
	}
	sort.Strings(extra)
	for _, name := range extra {
		id := c.ID("", "room:"+name)
		c.roomID[name] = id
		t.add(id, name, "General", 1, c.MigrationTS)
	}

	if needFallback {
		id := c.ID("", "room:"+fallbackRoomName)
		c.fallbackRoomID = id
		c.roomID[fallbackRoomName] = id
		t.add(id, fallbackRoomName, "General", 1, c.MigrationTS)
	}

	sec.Counts(t.len(), t.len(), 0)
	switch {
	case len(extra) > 0:
		sec.Issue(c.Report, "%d appointment room name(s) not in the seed were created as new 'General' rooms: %v.", len(extra), extra)
	case needFallback:
		sec.Note("All named appointment rooms are provided by the seed.")
	default:
		sec.Note("All appointment rooms are provided by the seed; nothing to add.")
	}
	if needFallback {
		sec.Note("Some appointments had a blank room; created an 'Unassigned' room to hold them.")
	}
	return t, nil
}

func loadProcedureCodeLabels(c *Context) (map[string]string, error) {
	f, err := c.File("categories.csv")
	if err != nil {
		return nil, err
	}
	cCode, cName := f.Col("id"), f.Col("name")
	labels := map[string]string{}
	for _, row := range f.Rows {
		code := csvutil.Get(row, cCode)
		name := csvutil.Get(row, cName)
		if code == "" || code == "0" || name == "" {
			continue
		}
		if _, ok := labels[code]; !ok {
			labels[code] = name
		}
	}
	return labels, nil
}

func migrateProducts(c *Context) (cats, products, prices *rowset, err error) {
	f, err := c.File("products.csv")
	if err != nil {
		return nil, nil, nil, err
	}
	sec := c.Report.Section("products")

	catID := c.ID("", "product_category:Products")
	cats = newRowset("id", "name")
	cats.add(catID, "Products")

	products = newRowset("id", "name", "category_id", "quantity", "min_threshold")
	prices = newRowset("id", "product_id", "price", "is_active")

	cID, cName := f.Col("id"), f.Col("name")
	cQty, cPrice := f.Col("quantity"), f.Col("unitprice")

	read, badQty, badPrice := 0, 0, 0
	idSeen := map[string]bool{}
	for _, row := range f.Rows {
		id := csvutil.Get(row, cID)
		name := csvutil.Get(row, cName)
		if id == "" || id == "0" || name == "" {
			continue
		}
		if idSeen[id] {
			continue
		}
		idSeen[id] = true
		read++

		qty, okq := conv.Int(csvutil.Get(row, cQty), 0)
		if !okq {
			badQty++
		}
		price, okp := conv.Float(csvutil.Get(row, cPrice), 0)
		if !okp {
			badPrice++
		}

		products.add(id, name, catID, qty, 0)
		prices.add(c.ID("", "product_price:"+id), id, store.Round2(price), 1)
	}

	sec.Counts(read, products.len(), 0)
	sec.Note("Products are not seeded; imported from products.csv keeping their CSV ids. Category/price rows use deterministic uuids.")
	sec.Note("min_threshold defaulted to 0 (absent in source); unitprice -> active product_price.")
	if badQty > 0 {
		sec.Issue(c.Report, "%d products had a non-numeric quantity (defaulted to 0).", badQty)
	}
	if badPrice > 0 {
		sec.Issue(c.Report, "%d products had a non-numeric price (defaulted to 0).", badPrice)
	}
	return cats, products, prices, nil
}
