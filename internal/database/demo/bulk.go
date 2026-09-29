package demo

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Bulk demo data: ~140 randomized patients created across the last ~6 months
// and several hundred appointments / invoices / payments / expenses / supplier
// purchases / gift cards spread over that window, so the analytics dashboard
// looks like a clinic that has been running for a while. Everything is random
// but seeded (demoCtx.rng) so runs are reproducible. Transactions are inserted
// raw and balances are recomputed once at the end (recalc balances step).

type pricedItem struct {
	id    string
	price float64
}

func seedBulkPatients(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	const count = 140

	firstM := []string{"Alexander", "Benjamin", "Christopher", "Daniel", "Edward", "Felix", "Gabriel", "Henry", "Ian",
		"Julian", "Lucas", "Matthew", "Nicholas", "Oliver", "Philip", "Quinn", "Robert", "Samuel", "Thomas",
		"Victor", "William", "Zachary", "Arthur", "Brian", "Colin", "David", "Elliott", "Francis"}
	firstF := []string{"Alice", "Beatrice", "Clara", "Eleanor", "Fiona", "Harriet", "Hannah", "Iris", "Julia",
		"Katherine", "Lucy", "Margaret", "Nora", "Olivia", "Penelope", "Rose", "Sophia", "Theresa", "Victoria",
		"Wendy", "Amelia", "Charlotte", "Miriam", "Emma", "Georgia", "Helen", "Isla", "Jane"}
	last := []string{"Adler", "Bennett", "Carter", "Donovan", "Evans", "Foster", "Gibson", "Hayes", "Ingram",
		"Jenkins", "Keller", "Lambert", "Monroe", "Navarro", "Palmer", "Parker", "Quincy", "Reynolds",
		"Sinclair", "Sterling", "Townsend", "Underwood", "Vance", "Wallace", "York", "Zimmerman", "Mercer", "North"}
	// Beirut weighted heavier so "Top Cities" has a clear leader.
	cities := []string{"Beirut", "Beirut", "Beirut", "Beirut", "Jounieh", "Jounieh", "Tripoli",
		"Tripoli", "Jbeil", "Zahle", "Saida", "Batroun", "Baabda"}
	referrals := []string{"Instagram", "Instagram", "Instagram", "Instagram", "Google", "Google",
		"Google", "TikTok", "TikTok", "Friend referral", "Friend referral", "Walk-in", "Doctor referral", ""}
	bloods := []string{"A+", "O+", "B+", "AB+", "A-", "O-", "B-", "AB-"}

	lebanonID := c.countries["Lebanon"]
	year := time.Now().UTC().Year()

	for i := 0; i < count; i++ {
		female := c.chance(0.55)
		gender := "Male"
		first := c.pick(firstM)
		if female {
			gender = "Female"
			first = c.pick(firstF)
		}
		lastN := c.pick(last)
		age := c.between(18, 70)
		dob := fmt.Sprintf("%04d-%02d-%02d", year-age, c.between(1, 12), c.between(1, 28))

		city := c.pick(cities)
		cityID := c.cities[city]
		if cityID == "" {
			cityID = c.cities["Beirut"]
		}

		off := -c.between(2, 200)
		createdAt := timeAt(off, c.between(8, 18), c.between(0, 59))
		id := newID()
		contact := fmt.Sprintf("+1 555 01%02d", c.between(0, 99))
		weight := float64(c.between(50, 95))
		height := float64(c.between(155, 190))

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO patients (id, first_name, middle_name, last_name, gender, date_of_birth,
				contact, email, emergency_contact_name, emergency_contact_phone,
				weight, height, blood_type, country_id, city_id, address, referral_id, referral_source, notes,
				created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,?,?,?,?)`,
			id, first, "", lastN, gender, dob,
			contact, "", "", "",
			weight, height, c.pick(bloods), lebanonID, cityID, "", c.pick(referrals), "",
			createdAt, createdAt,
		); err != nil {
			return err
		}
		bal, err := createBalance(ctx, tx, c, "patient", id, first+" "+lastN)
		if err != nil {
			return err
		}
		c.patientBal[id] = bal
		c.bulkPatients = append(c.bulkPatients, bulkPatient{id: id, bal: bal, createdOff: off})
	}
	return nil
}

func seedBulkHistory(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	procs, err := loadPriced(ctx, tx,
		`SELECT p.id, pr.price FROM procedures p
		 JOIN procedure_prices pr ON pr.procedure_id = p.id AND pr.is_active = 1
		 WHERE pr.price > 0`)
	if err != nil {
		return err
	}
	products, err := loadPriced(ctx, tx,
		`SELECT p.id, pr.price FROM products p
		 JOIN product_prices pr ON pr.product_id = p.id AND pr.is_active = 1`)
	if err != nil {
		return err
	}
	if len(procs) == 0 || len(c.bulkPatients) == 0 {
		return nil
	}

	var offers []string
	if rows, err := tx.QueryContext(ctx, `SELECT id FROM discounts WHERE discount_type = 'offer'`); err == nil {
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				offers = append(offers, id)
			}
		}
		rows.Close()
	}

	performers := append([]string{}, c.doctorIDs...)
	for _, idx := range []int{2, 3} { // laser tech + nurse, if present
		if idx < len(c.employeeIDs) {
			performers = append(performers, c.employeeIDs[idx])
		}
	}

	var roomIDs []string
	for _, id := range c.rooms {
		roomIDs = append(roomIDs, id)
	}

	methods := []string{"cash", "cash", "cash", "card", "card", "transfer"}
	giftValues := []string{"50", "100", "150", "200"}

	if err := generateAppointments(ctx, tx, c, procs, products, offers, performers, roomIDs, methods); err != nil {
		return err
	}
	if err := generateExpenses(ctx, tx, c); err != nil {
		return err
	}
	if err := generateSupplierPurchases(ctx, tx, c, products, methods); err != nil {
		return err
	}
	return generateGiftCards(ctx, tx, c, giftValues)
}

func generateAppointments(ctx context.Context, tx *sql.Tx, c *demoCtx, procs, products []pricedItem, offers, performers, roomIDs, methods []string) error {
	for day := -180; day <= 0; day++ {
		wd := weekdayOf(day)
		if wd == time.Sunday {
			continue
		}
		n := c.between(2, 6)
		if wd == time.Saturday {
			n = c.between(1, 3)
		}
		for k := 0; k < n; k++ {
			pat, ok := eligiblePatient(c, day)
			if !ok {
				continue
			}
			status := apptStatus(c, day)
			hour := c.between(9, 17)
			minute := 0
			if c.chance(0.5) {
				minute = 30
			}
			dur := []int{30, 45, 60, 90}[c.rng.Intn(4)]
			start := timeAt(day, hour, minute)
			end := addMinutes(day, hour, minute, dur)

			aptID := newID()
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, notes, cancel_notes, completion_notes, created_at, updated_at)
				 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
				aptID, pat.id, c.pick(roomIDs), start, end, status, "", "", "", start, start,
			); err != nil {
				return err
			}

			// Build the line items: usually a procedure (+ optional product),
			// occasionally a retail-only pickup, rarely a gift card sale.
			var items []invoiceItem
			retailOnly := c.chance(0.12) && len(products) > 0
			if retailOnly {
				for j := 0; j < c.between(1, 2); j++ {
					p := products[c.rng.Intn(len(products))]
					items = append(items, invoiceItem{itemType: "product", itemID: p.id, qty: 1, amount: p.price, finalAmount: p.price})
				}
			} else {
				pr := procs[c.rng.Intn(len(procs))]
				items = append(items, invoiceItem{itemType: "procedure", itemID: pr.id, qty: 1, amount: pr.price, finalAmount: pr.price})
				var assigned any
				if len(performers) > 0 {
					assigned = performers[c.rng.Intn(len(performers))]
				}
				if _, err := tx.ExecContext(ctx,
					`INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id, assigned_to_id, notes, created_at, updated_at)
					 VALUES (?,?,?,?,?,?,?,?)`,
					newID(), pat.id, pr.id, aptID, assigned, "", start, start,
				); err != nil {
					return err
				}
				if c.chance(0.25) && len(products) > 0 {
					p := products[c.rng.Intn(len(products))]
					items = append(items, invoiceItem{itemType: "product", itemID: p.id, qty: 1, amount: p.price, finalAmount: p.price})
				}
			}
			if c.chance(0.05) {
				g := float64([]int{50, 100, 150}[c.rng.Intn(3)])
				items = append(items, invoiceItem{itemType: "gift", itemID: "", qty: 1, amount: g, finalAmount: g})
			}

			// Only invoice realized visits (and retail pickups); cancelled /
			// rescheduled / plain scheduled appts carry no charge.
			invoice := status == "Completed" || status == "In-Progress" || (retailOnly && status != "Cancelled" && status != "Rescheduled")
			if !invoice || len(items) == 0 {
				continue
			}

			discountID := ""
			if c.chance(0.12) && len(offers) > 0 {
				discountID = offers[c.rng.Intn(len(offers))]
			}
			invID, total, err := createInvoice(ctx, tx, c, pat.bal, items, discountID, start)
			if err != nil {
				return err
			}
			if err := insertTxn(ctx, tx, c, c.selfBalanceID, pat.bal, total, "charge", "other", "invoice", invID, "Invoice charge", start); err != nil {
				return err
			}

			if pay := paymentAmount(c, status, total); pay > 0 {
				if err := insertTxn(ctx, tx, c, pat.bal, c.selfBalanceID, pay, "payment", c.pick(methods), "", "", "Patient payment", start); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func generateExpenses(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	if len(c.expenseIDs) < 4 {
		return nil
	}
	for m := 1; m <= 6; m++ {
		dayOff := -(m*30) + c.between(2, 25)
		if dayOff >= 0 {
			dayOff = -c.between(1, 25)
		}
		at := timeAt(dayOff, 9, 0)
		monthly := []struct {
			idx    int
			amount float64
			desc   string
		}{
			{0, 2400, "Monthly clinic rent"},
			{1, float64(c.between(380, 650)), "Utilities settlement"},
			{2, float64(c.between(400, 1200)), "Marketing campaign"},
			{3, float64(c.between(120, 260)), "Medical waste pickup"},
		}
		for _, e := range monthly {
			bal := c.expenseBal[c.expenseIDs[e.idx]]
			id := c.expenseIDs[e.idx]
			if err := insertTxn(ctx, tx, c, bal, c.selfBalanceID, e.amount, "charge", "other", "expense", id, e.desc, at); err != nil {
				return err
			}
			if err := insertTxn(ctx, tx, c, c.selfBalanceID, bal, e.amount, "payment", "transfer", "expense", id, e.desc, at); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateSupplierPurchases(ctx context.Context, tx *sql.Tx, c *demoCtx, products []pricedItem, methods []string) error {
	if len(c.supplierIDs) == 0 || len(products) == 0 {
		return nil
	}
	for m := 1; m <= 6; m++ {
		for p := 0; p < c.between(1, 2); p++ {
			dayOff := -(m*30) + c.between(2, 25)
			if dayOff >= 0 {
				dayOff = -c.between(1, 25)
			}
			at := timeAt(dayOff, 10, 0)
			supplierID := c.supplierIDs[c.rng.Intn(len(c.supplierIDs))]
			supplierBal := c.supplierBal[supplierID]

			var items []invoiceItem
			for j := 0; j < c.between(2, 3); j++ {
				pr := products[c.rng.Intn(len(products))]
				cost := pr.price * 0.55 // wholesale ~ 55% of retail
				items = append(items, invoiceItem{itemType: "product", itemID: pr.id, qty: c.between(4, 20), amount: cost, finalAmount: cost})
			}
			invID, total, err := createSupplierInvoice(ctx, tx, c, supplierBal, items, at)
			if err != nil {
				return err
			}
			if err := insertTxn(ctx, tx, c, supplierBal, c.selfBalanceID, total, "charge", "other", "invoice", invID, "Supplier invoice charge", at); err != nil {
				return err
			}
			pay := total
			if r := c.rng.Float64(); r < 0.2 {
				pay = 0 // unpaid, left payable
			} else if r < 0.5 {
				pay = total * (0.4 + 0.4*c.rng.Float64()) // partial
			}
			if pay > 0 {
				if err := insertTxn(ctx, tx, c, c.selfBalanceID, supplierBal, pay, "payment", c.pick(methods), "", "", "Supplier payment", at); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func generateGiftCards(ctx context.Context, tx *sql.Tx, c *demoCtx, values []string) error {
	for i := 0; i < 7; i++ {
		off := -c.between(5, 170)
		createdAt := timeAt(off, 12, 0)
		var redeemedAt any
		if c.chance(0.4) {
			redeemedAt = timeAt(off+c.between(1, 20), 12, 0)
		}
		value := c.pick(values)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO discounts (id, name, description, discount_type, value_type, value, code, redeemed_at, is_active, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,1,?,?)`,
			newID(), "Gift Card $"+value, "Demo gift card", "gift", "fixed", value,
			fmt.Sprintf("GIFT-%05d", c.between(10000, 99999)), redeemedAt, createdAt, createdAt,
		); err != nil {
			return err
		}
	}
	return nil
}

// small helpers

func loadPriced(ctx context.Context, tx *sql.Tx, query string) ([]pricedItem, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pricedItem
	for rows.Next() {
		var it pricedItem
		if err := rows.Scan(&it.id, &it.price); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func eligiblePatient(c *demoCtx, day int) (bulkPatient, bool) {
	var elig []bulkPatient
	for _, p := range c.bulkPatients {
		if p.createdOff <= day {
			elig = append(elig, p)
		}
	}
	if len(elig) == 0 {
		return bulkPatient{}, false
	}
	return elig[c.rng.Intn(len(elig))], true
}

func apptStatus(c *demoCtx, day int) string {
	if day >= 0 {
		switch r := c.rng.Float64(); {
		case r < 0.5:
			return "Scheduled"
		case r < 0.7:
			return "In-Progress"
		default:
			return "Completed"
		}
	}
	switch r := c.rng.Float64(); {
	case r < 0.85:
		return "Completed"
	case r < 0.93:
		return "Cancelled"
	default:
		return "Rescheduled"
	}
}

func paymentAmount(c *demoCtx, status string, total float64) float64 {
	if status == "In-Progress" {
		if c.chance(0.5) {
			return total
		}
		return 0
	}
	// Completed / retail.
	switch r := c.rng.Float64(); {
	case r < 0.7:
		return total
	case r < 0.9:
		return total * (0.4 + 0.4*c.rng.Float64()) // partial
	default:
		return 0
	}
}

func addMinutes(day, hour, minute, dur int) string {
	total := minute + dur
	return timeAt(day, hour+total/60, total%60)
}
