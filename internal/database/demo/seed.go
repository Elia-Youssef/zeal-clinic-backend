// Package demo populates a small, self-contained dummy dataset for development
// and screenshots. It runs as a standalone goose migration with its own
// version table (goose_demo_version) so the main migration flow stays clean.
package demo

import (
	"context"
	"database/sql"
	"fmt"

	"clinic-api/internal/auth"

	"github.com/pressly/goose/v3"
)

// Migrations returns the demo migrations to feed into a goose Provider. We
// register them explicitly (instead of via init() into the global registry)
// so they don't get pulled into the main schema migration runner.
func Migrations() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(1, &goose.GoFunc{RunTx: seedDemo}, nil),
	}
}

func seedDemo(ctx context.Context, tx *sql.Tx) error {
	c, err := newDemoCtx(ctx, tx)
	if err != nil {
		return fmt.Errorf("demo context: %w", err)
	}

	steps := []struct {
		name string
		fn   func(context.Context, *sql.Tx, *demoCtx) error
	}{
		{"allergies", seedAllergies},
		{"product categories & products", seedProducts},
		{"medicines", seedMedicines},
		{"employees", seedEmployees},
		{"schedules", seedSchedules},
		{"suppliers", seedSuppliers},
		{"patients", seedPatients},
		{"patient links", seedPatientLinks},
		{"prescriptions", seedPrescriptions},
		{"discounts", seedDiscounts},
		{"appointments & invoices", seedAppointmentsAndInvoices},
		{"patient procedures", seedPatientProcedures},
		{"bookings", seedBookings},
		{"notifications", seedNotifications},
	}
	for _, s := range steps {
		if err := s.fn(ctx, tx, c); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	return nil
}

// Reference data

func seedAllergies(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	allergies := []string{"Penicillin", "Latex", "Lidocaine", "Pollen", "Aspirin", "Peanuts"}
	for _, name := range allergies {
		id := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO allergies (id, name, description, created_at) VALUES (?,?,?,?)`,
			id, name, "", c.now,
		); err != nil {
			return err
		}
		c.allergies[name] = id
	}
	return nil
}

func seedProducts(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	categories := map[string]string{}
	for _, name := range []string{"Skincare", "Injectables"} {
		id := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO product_categories (id, name, description, parent_id, created_at) VALUES (?,?,?,?,?)`,
			id, name, "", "", c.now,
		); err != nil {
			return err
		}
		categories[name] = id
	}

	products := []struct {
		name, category string
		quantity, min  int
		price          float64
	}{
		{"Vitamin C Serum", "Skincare", 20, 5, 45},
		{"Hyaluronic Acid Cream", "Skincare", 15, 5, 55},
		{"Botox Vial 100u", "Injectables", 8, 2, 280},
		{"HA Filler 1ml", "Injectables", 12, 3, 160},
	}
	for _, p := range products {
		id := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO products (id, name, category_id, quantity, min_threshold, created_at) VALUES (?,?,?,?,?,?)`,
			id, p.name, categories[p.category], p.quantity, p.min, c.now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO product_prices (id, product_id, price, is_active, created_at) VALUES (?,?,?,1,?)`,
			newID(), id, p.price, c.now,
		); err != nil {
			return err
		}
		c.products[p.name] = id
	}
	return nil
}

func seedMedicines(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	meds := []string{
		"Paracetamol 500mg", "Ibuprofen 400mg", "Amoxicillin 500mg",
		"Cephalexin 500mg", "Arnica Gel", "Cetirizine 10mg",
	}
	for _, name := range meds {
		id := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO medicines (id, name, description, created_at) VALUES (?,?,?,?)`,
			id, name, "", c.now,
		); err != nil {
			return err
		}
		c.medicines[name] = id
	}
	return nil
}

// People

func seedEmployees(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	type emp struct {
		first, last, role, contact, email, dob, employment string
		salary                                             float64
		isDoctor                                           bool
		username, displayName, userRole                    string // username == "" means no user account
	}
	employees := []emp{
		{"Julian", "Vance", "Plastic Surgeon", "+1 555 0107", "julian@example.org", "1978-04-12", "Full-time",
			5000, true, "jvance", "Dr. Julian Vance", "admin"},
		{"Maya", "Aoun", "Nurse", "+1 555 0113", "maya@example.org", "1990-07-17", "Full-time",
			1500, false, "maoun", "Maya Aoun", "user"},
		{"Rita", "Saad", "Receptionist", "+1 555 0114", "rita@example.org", "1992-11-30", "Part-time",
			900, false, "", "", ""},
	}

	for _, e := range employees {
		userID := ""
		if e.username != "" {
			userID = newID()
			hash, err := auth.HashPassword("demo123")
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO users (id, username, password_hash, display_name, role, is_active, created_at, updated_at)
				 VALUES (?,?,?,?,?,1,?,?)`,
				userID, e.username, hash, e.displayName, e.userRole, c.now, c.now,
			); err != nil {
				return err
			}
		}

		empID := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO employees (id, user_id, first_name, last_name, role, contact, email, date_of_birth, employment_type, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			empID, userID, e.first, e.last, e.role, e.contact, e.email, e.dob, e.employment, c.now, c.now,
		); err != nil {
			return err
		}

		balID, err := createBalance(ctx, tx, c, "employee", empID, e.first+" "+e.last)
		if err != nil {
			return err
		}
		c.employeeBal[empID] = balID

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO employee_salaries (id, employee_id, amount, currency_id, is_active, effective_date, notes, created_at, updated_at)
			 VALUES (?,?,?,?,1,?,?,?,?)`,
			newID(), empID, e.salary, c.currencyID, c.today, "Current base salary", c.now, c.now,
		); err != nil {
			return err
		}

		c.employeeIDs = append(c.employeeIDs, empID)
		if e.isDoctor {
			c.doctorIDs = append(c.doctorIDs, empID)
		}
	}
	return nil
}

func seedSchedules(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	for _, empID := range c.employeeIDs {
		for day := 1; day <= 5; day++ {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO schedule_availability (id, employee_id, day_of_week, start_time, end_time, effective_date, created_at)
				 VALUES (?,?,?,?,?,?,?)`,
				newID(), empID, day, "09:00", "18:00", c.today, c.now,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedSuppliers(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	suppliers := []struct{ name, contact, email, address string }{
		{"MedSupply Co", "+961 1 234 567", "orders@example.com", "Beirut, Lebanon"},
		{"DermaPharma", "+961 1 345 678", "info@example.org", "Jounieh, Lebanon"},
	}
	for _, s := range suppliers {
		id := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO suppliers (id, name, contact, email, address, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?)`,
			id, s.name, s.contact, s.email, s.address, c.now, c.now,
		); err != nil {
			return err
		}
		balID, err := createBalance(ctx, tx, c, "supplier", id, s.name)
		if err != nil {
			return err
		}
		c.supplierBal[id] = balID
	}
	return nil
}

func seedPatients(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	type pat struct {
		first, last, gender, dob, contact, email, blood, city, address, source, notes string
		weight, height                                                                 float64
	}
	patients := []pat{
		{"Nour", "Abou Khalil", "Female", "1991-03-14", "+1 555 0118", "nour@example.com",
			"A+", "Beirut", "Achrafieh, Rue Sursock", "Instagram", "Regular client",
			58, 165},
		{"Rami", "Chahine", "Male", "1985-08-22", "+1 555 0120", "rami@example.com",
			"O+", "Jounieh", "Kaslik, Main Road", "Google", "Prefers Dr. Julian",
			82, 180},
		{"Lea", "Sassine", "Female", "1994-12-01", "+1 555 0134", "lea@example.com",
			"B+", "Beirut", "Gemmayzeh", "Friend referral", "",
			54, 168},
		{"Karim", "Fakhoury", "Male", "1979-05-19", "+1 555 0136", "karim@example.com",
			"A-", "Baabda", "Rabieh, Block 4", "Walk-in", "Botox for migraine",
			90, 183},
		{"Yasmina", "Daou", "Female", "1996-02-28", "+1 555 0138", "yasmina@example.com",
			"AB+", "Tripoli", "Dam wel Farez", "Instagram", "First-time lip filler",
			60, 170},
		{"Elie", "Zogheib", "Male", "1982-07-09", "+1 555 0140", "elie@example.com",
			"O-", "Jbeil", "Old Souk", "TikTok", "",
			78, 178},
	}

	lebanonID := c.countries["Lebanon"]
	for _, p := range patients {
		id := newID()
		cityID := c.cities[p.city]
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO patients (id, first_name, middle_name, last_name, gender, date_of_birth,
				contact, email, emergency_contact_name, emergency_contact_phone,
				weight, height, blood_type, country_id, city_id, address, referral_id, referral_source, notes,
				created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,?,?,?,?)`,
			id, p.first, "", p.last, p.gender, p.dob,
			p.contact, p.email, "", "",
			p.weight, p.height, p.blood, lebanonID, cityID, p.address, p.source, p.notes,
			c.now, c.now,
		); err != nil {
			return err
		}
		balID, err := createBalance(ctx, tx, c, "patient", id, p.first+" "+p.last)
		if err != nil {
			return err
		}
		c.patientBal[id] = balID
		c.patientIDs = append(c.patientIDs, id)
	}
	return nil
}

func seedPatientLinks(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	// Index-based map: patientIDs[0]=Nour, [1]=Rami, [2]=Lea, [3]=Karim, [4]=Yasmina, [5]=Elie
	allergyLinks := map[int][]string{
		0: {"Penicillin", "Latex"},
		3: {"Aspirin"},
		4: {"Pollen"},
	}
	for idx, names := range allergyLinks {
		for _, name := range names {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO patient_allergies (id, patient_id, allergy_id, notes, created_at) VALUES (?,?,?,?,?)`,
				newID(), c.patientIDs[idx], c.allergies[name], "", c.now,
			); err != nil {
				return err
			}
		}
	}

	medLinks := map[int][]string{
		2: {"Cetirizine 10mg"},
		3: {"Paracetamol 500mg"},
	}
	for idx, names := range medLinks {
		for _, name := range names {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO patient_medicines (id, patient_id, medicine_id, is_active, notes, created_at)
				 VALUES (?,?,?,1,?,?)`,
				newID(), c.patientIDs[idx], c.medicines[name], "Ongoing", c.now,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedPrescriptions(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	if len(c.doctorIDs) == 0 {
		return nil
	}
	doctor := c.doctorIDs[0]

	type rx struct {
		patientIdx       int
		startOff, endOff int
		meds             []struct{ name, instructions, status string }
	}
	prescriptions := []rx{
		{0, -10, -3, []struct{ name, instructions, status string }{
			{"Amoxicillin 500mg", "1 cap three times daily for 7 days", "completed"},
			{"Arnica Gel", "Apply 3-4 times daily to bruising", "completed"},
		}},
		{3, -5, 2, []struct{ name, instructions, status string }{
			{"Cephalexin 500mg", "1 cap three times daily for 7 days", "active"},
			{"Ibuprofen 400mg", "1 tab every 8h with food", "active"},
		}},
	}
	for _, p := range prescriptions {
		pid := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO prescriptions (id, patient_id, prescribed_by_id, start_date, end_date, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?)`,
			pid, c.patientIDs[p.patientIdx], doctor, dateOffset(p.startOff), dateOffset(p.endOff), c.now, c.now,
		); err != nil {
			return err
		}
		for _, m := range p.meds {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO prescription_medicines (id, medicine_id, prescription_id, instructions, status, created_at)
				 VALUES (?,?,?,?,?,?)`,
				newID(), c.medicines[m.name], pid, m.instructions, m.status, c.now,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedDiscounts(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	startStr := dateOffset(-15)
	endStr := dateOffset(30)

	// Offer: 20% off any Botox procedure.
	offerID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, max_usages, current_usages, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,NULL,0,?,?,1,?,?)`,
		offerID, "Spring Botox -20%", "20% off any Botox procedure",
		"offer", "percentage", 20.0, startStr, endStr, c.now, c.now,
	); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM procedures WHERE name LIKE 'Botox%'`)
	if err != nil {
		return err
	}
	var botoxIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		botoxIDs = append(botoxIDs, id)
	}
	rows.Close()
	for _, pid := range botoxIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO discount_items (id, discount_id, item_type, item_id, created_at) VALUES (?,?,?,?,?)`,
			newID(), offerID, "procedure", pid, c.now,
		); err != nil {
			return err
		}
	}

	// Voucher: $50 off, 5 codes (one used to demo current_usages).
	voucherID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, max_usages, current_usages, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,1,?,?)`,
		voucherID, "$50 off Skin Boosters", "Single-use vouchers, $50 off any skin booster",
		"voucher", "fixed", 50.0, 5, 1, startStr, endStr, c.now, c.now,
	); err != nil {
		return err
	}
	codes := []struct {
		code string
		used bool
	}{
		{"SB-DEMO-001", true},
		{"SB-DEMO-002", false},
		{"SB-DEMO-003", false},
		{"SB-DEMO-004", false},
		{"SB-DEMO-005", false},
	}
	for _, v := range codes {
		used := 0
		if v.used {
			used = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO vouchers (id, discount_id, code, is_used, created_at) VALUES (?,?,?,?,?)`,
			newID(), voucherID, v.code, used, c.now,
		); err != nil {
			return err
		}
	}
	return nil
}

// Appointments + invoices + payments

func seedAppointmentsAndInvoices(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	if len(c.doctorIDs) == 0 {
		return nil
	}
	room1 := c.rooms["Room 1"]
	room2 := c.rooms["Room 2"]

	type apt struct {
		patientIdx                int
		room                      string
		dayOff, hour              int
		duration                  int
		status                    string
		procedureName             string
		productNames              []string
		paymentAmount             float64
		paymentMethod             string
		applyOfferToBotox         bool
		notes, completionNotes    string
	}
	appointments := []apt{
		// Past: completed, fully paid in cash.
		{0, room1, -7, 10, 60, "Completed", "Botox Full", nil, 220, "cash", false, "Routine touch-up", "Done; client happy"},
		// Past: completed, 20% Botox offer applied, paid by card.
		{3, room2, -5, 14, 60, "Completed", "Botox Migraine", nil, 320, "card", true, "Migraine session", ""},
		// Past: completed, partial payment ($150 of $250 lips filler).
		{2, room1, -3, 11, 45, "Completed", "Lips", nil, 150, "cash", false, "First filler", ""},
		// Today: scheduled (no invoice).
		{4, room1, 0, 16, 60, "Scheduled", "", nil, 0, "", false, "Lip filler consult", ""},
		// Tomorrow: scheduled retail order (products only, charge invoice, no payment).
		{1, room2, 1, 12, 30, "Scheduled", "", []string{"Vitamin C Serum", "Hyaluronic Acid Cream"}, 0, "", false, "Retail pickup", ""},
		// +3 days: scheduled.
		{5, room1, 3, 15, 60, "Scheduled", "", nil, 0, "", false, "", ""},
	}

	for _, a := range appointments {
		patientID := c.patientIDs[a.patientIdx]
		patientBal := c.patientBal[patientID]
		startStr := timeAt(a.dayOff, a.hour, 0)
		endStr := timeAt(a.dayOff, a.hour+(a.duration/60), a.duration%60)

		aptID := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status, notes, cancel_notes, completion_notes, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			aptID, patientID, a.room, startStr, endStr, a.status, a.notes, "", a.completionNotes, c.now, c.now,
		); err != nil {
			return err
		}

		// Build invoice items + totals. Skip when there's nothing to charge.
		var items []invoiceItem
		if a.procedureName != "" {
			procID := c.procedures[a.procedureName]
			price, err := procedurePrice(ctx, tx, procID)
			if err != nil {
				return err
			}
			final := price
			var discountID string
			var discountValue float64
			if a.applyOfferToBotox {
				discountID, discountValue = botoxOffer(ctx, tx)
				if discountID != "" {
					final = price * (1 - discountValue/100)
				}
			}
			items = append(items, invoiceItem{
				itemType: "procedure", itemID: procID, qty: 1,
				amount: price, finalAmount: final,
				discountID: discountID, discountValue: discountValue,
			})
		}
		for _, name := range a.productNames {
			pid := c.products[name]
			price, err := productPrice(ctx, tx, pid)
			if err != nil {
				return err
			}
			items = append(items, invoiceItem{
				itemType: "product", itemID: pid, qty: 1,
				amount: price, finalAmount: price,
			})
		}
		if len(items) == 0 {
			continue
		}

		invoiceID, total, err := createInvoice(ctx, tx, c, patientBal, items, aptID)
		if err != nil {
			return err
		}
		_ = invoiceID

		// Record the charge (no actual money moved).
		if err := recordTransaction(ctx, tx, c, c.selfBalanceID, patientBal, total,
			"charge", "other", "Invoice charge", startStr); err != nil {
			return err
		}
		// Record the payment when there is one.
		if a.paymentAmount > 0 {
			if err := recordTransaction(ctx, tx, c, patientBal, c.selfBalanceID, a.paymentAmount,
				"payment", a.paymentMethod, "Patient payment", startStr); err != nil {
				return err
			}
		}
	}
	return nil
}

type invoiceItem struct {
	itemType, itemID string
	qty              int
	amount           float64
	finalAmount      float64
	discountID       string
	discountValue    float64 // percentage value if discount is percent-typed
}

func createInvoice(ctx context.Context, tx *sql.Tx, c *demoCtx, patientBal string, items []invoiceItem, _ string) (string, float64, error) {
	var nextNumber int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(invoice_number), 0) + 1 FROM invoices`,
	).Scan(&nextNumber); err != nil {
		return "", 0, err
	}

	var amount, finalAmount float64
	for _, it := range items {
		amount += it.amount * float64(it.qty)
		finalAmount += it.finalAmount * float64(it.qty)
	}

	invID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, final_amount, currency_id, notes, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		invID, nextNumber, c.selfBalanceID, patientBal, amount, finalAmount, c.currencyID, "", c.adminUserID, c.now, c.now,
	); err != nil {
		return "", 0, err
	}

	for _, it := range items {
		itemID := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO invoice_items (id, invoice_id, item_type, item_id, quantity, amount, final_amount, notes, created_at)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			itemID, invID, it.itemType, it.itemID, it.qty, it.amount, it.finalAmount, "", c.now,
		); err != nil {
			return "", 0, err
		}
		if it.discountID != "" {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO invoice_item_discounts (id, invoice_item_id, discount_id, discount_value, created_at)
				 VALUES (?,?,?,?,?)`,
				newID(), itemID, it.discountID, it.discountValue, c.now,
			); err != nil {
				return "", 0, err
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE discounts SET current_usages = current_usages + 1, updated_at = ? WHERE id = ?`,
				c.now, it.discountID,
			); err != nil {
				return "", 0, err
			}
		}
	}
	return invID, finalAmount, nil
}

func procedurePrice(ctx context.Context, tx *sql.Tx, procID string) (float64, error) {
	var price float64
	err := tx.QueryRowContext(ctx,
		`SELECT price FROM procedure_prices WHERE procedure_id = ? AND is_active = 1 LIMIT 1`,
		procID,
	).Scan(&price)
	return price, err
}

func productPrice(ctx context.Context, tx *sql.Tx, prodID string) (float64, error) {
	var price float64
	err := tx.QueryRowContext(ctx,
		`SELECT price FROM product_prices WHERE product_id = ? AND is_active = 1 LIMIT 1`,
		prodID,
	).Scan(&price)
	return price, err
}

func botoxOffer(ctx context.Context, tx *sql.Tx) (string, float64) {
	var id string
	var value float64
	if err := tx.QueryRowContext(ctx,
		`SELECT id, value FROM discounts WHERE name = 'Spring Botox -20%' LIMIT 1`,
	).Scan(&id, &value); err != nil {
		return "", 0
	}
	return id, value
}

// Patient procedures

func seedPatientProcedures(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	if len(c.patientIDs) == 0 {
		return nil
	}
	procID := c.procedures["Face + Plasma (3 sessions)"]
	if procID == "" {
		return nil
	}

	// Patient 0 (Nour): an in-progress Morpheus8 face course.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO patient_procedures (id, patient_id, procedure_id, status, notes, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?)`,
		newID(), c.patientIDs[0], procID, "in-progress", "Morpheus8 face course", c.now, c.now,
	); err != nil {
		return err
	}
	return nil
}

// Bookings + notifications

func seedBookings(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	bookings := []struct {
		name, phone, email, source, category, service, date, atTime string
		duration                                                    int
		isNew                                                       bool
		status                                                      string
	}{
		{"Aya Daher", "+1 555 0190", "aya.d@example.com", "Instagram", "Face", "Lips Filler",
			dateOffset(1), "11:00", 45, true, "pending"},
		{"Hadi Trad", "+1 555 0191", "hadi.t@example.com", "Friend referral", "Botox", "Botox Full",
			dateOffset(7), "15:00", 60, false, "confirmed"},
	}
	for _, b := range bookings {
		isNew := 0
		if b.isNew {
			isNew = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bookings (id, client_name, client_phone, client_email, is_new_client, referral_source,
				service_category, service_name, preferred_date, preferred_time, duration_minutes, status, notes, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			newID(), b.name, b.phone, b.email, isNew, b.source, b.category, b.service,
			b.date, b.atTime, b.duration, b.status, "", c.now, c.now,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedNotifications(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	notifications := []struct {
		title, description, action string
		read                       bool
	}{
		{"Low stock: Botox Vial 100u", "Quantity 8 has dropped near min threshold.", "/inventory", false},
		{"New booking received", "Aya Daher requested an appointment.", "/bookings", false},
		{"Appointment scheduled", "Yasmina Daou is scheduled for today at 16:00.", "/appointments", true},
	}
	for _, n := range notifications {
		read := 0
		if n.read {
			read = 1
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO notifications (id, user_id, title, description, action, is_read, created_at)
			 VALUES (?,?,?,?,?,?,?)`,
			newID(), c.adminUserID, n.title, n.description, n.action, read, c.now,
		); err != nil {
			return err
		}
	}
	return nil
}
