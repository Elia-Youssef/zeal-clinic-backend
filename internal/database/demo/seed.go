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
		{"expenses", seedExpenses},
		{"patients", seedPatients},
		{"patient links", seedPatientLinks},
		{"prescriptions", seedPrescriptions},
		{"discounts", seedDiscounts},
		{"supplier invoices & expenses", seedSupplierInvoicesAndExpenses},
		{"appointments & invoices", seedAppointmentsAndInvoices},
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
	allergies := []string{
		"Penicillin", "Latex", "Lidocaine", "Pollen", "Aspirin", "Peanuts",
		"Shellfish", "Iodine", "Fragrance", "Retinoids",
	}
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
	for _, name := range []string{"Skincare", "Injectables", "Post Treatment", "Devices"} {
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
		{"Vitamin C Serum", "Skincare", 24, 5, 45},
		{"Hyaluronic Acid Cream", "Skincare", 18, 5, 55},
		{"Retinol Night Serum", "Skincare", 14, 4, 60},
		{"SPF 50 Mineral Cream", "Skincare", 32, 8, 38},
		{"Botox Vial 100u", "Injectables", 9, 3, 280},
		{"HA Filler 1ml", "Injectables", 16, 4, 160},
		{"Skin Booster 2ml", "Injectables", 10, 3, 190},
		{"Post Laser Repair Balm", "Post Treatment", 22, 6, 35},
		{"Cooling Gel Pack", "Post Treatment", 6, 8, 12},
		{"Microneedling Cartridge", "Devices", 40, 10, 9},
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
		"Valacyclovir 500mg", "Doxycycline 100mg", "Mupirocin Ointment",
		"Hydrocortisone 1% Cream",
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
		{"Lina", "Haddad", "Dermatologist", "+1 555 0108", "lina@example.org", "1984-09-03", "Full-time",
			4200, true, "lhaddad", "Dr. Lina Haddad", "user"},
		{"Tarek", "Mansour", "Laser Technician", "+1 555 0109", "tarek@example.org", "1988-01-21", "Full-time",
			1800, false, "tmansour", "Tarek Mansour", "user"},
		{"Maya", "Aoun", "Nurse", "+1 555 0113", "maya@example.org", "1990-07-17", "Full-time",
			1500, false, "maoun", "Maya Aoun", "user"},
		{"Rita", "Saad", "Receptionist", "+1 555 0114", "rita@example.org", "1992-11-30", "Part-time",
			900, false, "", "", ""},
		{"Samir", "Nassar", "Accountant", "+1 555 0115", "samir@example.org", "1986-05-25", "Part-time",
			1200, false, "", "", ""},
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
				`INSERT INTO schedule_availability (id, employee_id, day_of_week, start_time, end_time, start_date, created_at, updated_at)
				 VALUES (?,?,?,?,?,?,?,?)`,
				newID(), empID, day, "09:00", "18:00", c.today, c.now, c.now,
			); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO holidays (id, name, start_date, end_date, notes, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		newID(), "Clinic Maintenance Day", dateOffset(14), dateOffset(14), "Demo closure for maintenance", c.adminUserID, c.now, c.now,
	); err != nil {
		return err
	}
	if len(c.employeeIDs) > 1 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO employee_vacations (id, employee_id, start_date, end_date, start_time, end_time, status, notes, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			newID(), c.employeeIDs[1], dateOffset(5), dateOffset(5), "13:00", "18:00", "accepted", "Conference afternoon", c.now, c.now,
		); err != nil {
			return err
		}
	}
	if len(c.employeeIDs) > 3 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO employee_vacations (id, employee_id, start_date, end_date, start_time, end_time, status, notes, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			newID(), c.employeeIDs[3], dateOffset(9), dateOffset(11), "", "", "pending", "Family trip request", c.now, c.now,
		); err != nil {
			return err
		}
	}
	return nil
}

func seedSuppliers(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	suppliers := []struct{ name, contact, email, address string }{
		{"MedSupply Co", "+961 1 234 567", "orders@example.com", "Beirut, Lebanon"},
		{"DermaPharma", "+961 1 345 678", "info@example.org", "Jounieh, Lebanon"},
		{"LaserCare Parts", "+961 1 456 789", "support@lasercare.example", "Dbayeh, Lebanon"},
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
		c.supplierIDs = append(c.supplierIDs, id)
	}
	return nil
}

func seedExpenses(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	expenses := []struct {
		name, notes string
	}{
		{"Rent", "Monthly clinic rent"},
		{"Utilities", "Electricity, generator and water"},
		{"Marketing", "Social ads and campaign production"},
		{"Medical Waste", "Biohazard disposal service"},
	}
	for _, e := range expenses {
		id := newID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO expenses (id, name, notes, created_at, updated_at)
			 VALUES (?,?,?,?,?)`,
			id, e.name, e.notes, c.now, c.now,
		); err != nil {
			return err
		}
		balID, err := createBalance(ctx, tx, c, "expense", id, e.name)
		if err != nil {
			return err
		}
		c.expenseBal[id] = balID
		c.expenseIDs = append(c.expenseIDs, id)
	}
	return nil
}

func seedPatients(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	type pat struct {
		first, last, gender, dob, contact, email, blood, city, address, source, notes string
		weight, height                                                                float64
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
		{"Mira", "Hobeika", "Female", "1989-10-12", "+1 555 0192", "mira@example.com",
			"A+", "Zahle", "Ksara", "Friend referral", "Interested in skin boosters",
			62, 166},
		{"Omar", "Karam", "Male", "1993-06-03", "+1 555 0193", "omar@example.com",
			"B-", "Saida", "Abra", "Google", "Laser hair removal package",
			84, 181},
		{"Sara", "Maalouf", "Female", "1976-01-26", "+1 555 0194", "sara@example.com",
			"O+", "Beirut", "Verdun", "Instagram", "Prefers morning appointments",
			67, 164},
		{"Hadi", "Nehme", "Male", "1987-09-18", "+1 555 0195", "hadi@example.com",
			"AB-", "Jounieh", "Haret Sakher", "Walk-in", "Post-surgery follow-up",
			88, 176},
		{"Dalia", "Sfeir", "Female", "1998-04-07", "+1 555 0196", "dalia@example.com",
			"A-", "Batroun", "Sea Road", "TikTok", "Sensitive skin",
			55, 162},
		{"Nabil", "Rahme", "Male", "1969-12-15", "+1 555 0197", "nabil@example.com",
			"O+", "Tripoli", "Mina", "Doctor referral", "Consultation required before surgery",
			92, 179},
	}

	lebanonID := c.countries["Lebanon"]
	for _, p := range patients {
		id := newID()
		cityID := c.cities[p.city]
		if cityID == "" {
			cityID = c.cities["Beirut"]
		}
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
		0:  {"Penicillin", "Latex"},
		3:  {"Aspirin"},
		4:  {"Pollen"},
		6:  {"Fragrance", "Retinoids"},
		8:  {"Lidocaine"},
		10: {"Shellfish", "Iodine"},
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
		2:  {"Cetirizine 10mg"},
		3:  {"Paracetamol 500mg"},
		6:  {"Hydrocortisone 1% Cream"},
		8:  {"Valacyclovir 500mg"},
		9:  {"Doxycycline 100mg"},
		10: {"Cetirizine 10mg"},
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
		meds             []struct{ name, instructions string }
	}
	prescriptions := []rx{
		{0, -10, -3, []struct{ name, instructions string }{
			{"Amoxicillin 500mg", "1 cap three times daily for 7 days"},
			{"Arnica Gel", "Apply 3-4 times daily to bruising"},
		}},
		{3, -5, 2, []struct{ name, instructions string }{
			{"Cephalexin 500mg", "1 cap three times daily for 7 days"},
			{"Ibuprofen 400mg", "1 tab every 8h with food"},
		}},
		{8, -2, 5, []struct{ name, instructions string }{
			{"Valacyclovir 500mg", "1 tab twice daily for 5 days"},
		}},
		{10, -20, -13, []struct{ name, instructions string }{
			{"Doxycycline 100mg", "1 cap daily after food for 7 days"},
			{"Mupirocin Ointment", "Apply thin layer twice daily"},
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
				`INSERT INTO prescription_medicines (id, medicine_id, prescription_id, instructions, created_at)
				 VALUES (?,?,?,?,?)`,
				newID(), c.medicines[m.name], pid, m.instructions, c.now,
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

	// Offer: 20% off, applied invoice-wide.
	springID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,1,?,?)`,
		springID, "Spring Botox -20%", "20% off selected Botox invoices",
		"offer", "percentage", 20.0, startStr, endStr, c.now, c.now,
	); err != nil {
		return err
	}

	// Offer: $50 off, applied invoice-wide.
	flatID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,1,?,?)`,
		flatID, "$50 off", "Flat $50 off any invoice",
		"offer", "fixed", 50.0, startStr, endStr, c.now, c.now,
	); err != nil {
		return err
	}

	// Demo gift card with a code (unredeemed).
	giftID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, code, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,1,?,?)`,
		giftID, "Demo Gift Card $100", "Redeemable gift card",
		"gift", "fixed", 100.0, "GIFT-DEMO-001", c.now, c.now,
	); err != nil {
		return err
	}
	return nil
}

// Appointments + invoices + payments

func seedSupplierInvoicesAndExpenses(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	supplierPurchases := []struct {
		supplierIdx   int
		items         []invoiceItem
		paymentAmount float64
		paymentMethod string
		dayOff        int
	}{
		{0, []invoiceItem{
			{itemType: "product", itemID: c.products["Botox Vial 100u"], qty: 4, amount: 210, finalAmount: 210},
			{itemType: "product", itemID: c.products["HA Filler 1ml"], qty: 8, amount: 120, finalAmount: 120},
		}, 1200, "transfer", -18},
		{1, []invoiceItem{
			{itemType: "product", itemID: c.products["Vitamin C Serum"], qty: 12, amount: 24, finalAmount: 24},
			{itemType: "product", itemID: c.products["SPF 50 Mineral Cream"], qty: 18, amount: 19, finalAmount: 19},
			{itemType: "product", itemID: c.products["Retinol Night Serum"], qty: 10, amount: 32, finalAmount: 32},
		}, 500, "card", -9},
		{2, []invoiceItem{
			{itemType: "product", itemID: c.products["Microneedling Cartridge"], qty: 50, amount: 5, finalAmount: 5},
			{itemType: "product", itemID: c.products["Cooling Gel Pack"], qty: 30, amount: 6, finalAmount: 6},
		}, 0, "", -4},
	}
	for _, p := range supplierPurchases {
		if p.supplierIdx >= len(c.supplierIDs) {
			continue
		}
		at := timeAt(p.dayOff, 9, 0)
		supplierBal := c.supplierBal[c.supplierIDs[p.supplierIdx]]
		invoiceID, total, err := createSupplierInvoice(ctx, tx, c, supplierBal, p.items, at)
		if err != nil {
			return err
		}
		if err := recordTransactionSource(ctx, tx, c, supplierBal, c.selfBalanceID, total,
			"charge", "other", "invoice", invoiceID, "Supplier invoice charge", at); err != nil {
			return err
		}
		if p.paymentAmount > 0 {
			if err := recordTransaction(ctx, tx, c, c.selfBalanceID, supplierBal, p.paymentAmount,
				"payment", p.paymentMethod, "Supplier payment", at); err != nil {
				return err
			}
		}
	}

	expensePayments := []struct {
		expenseIdx int
		amount     float64
		method     string
		dayOff     int
		desc       string
	}{
		{0, 2400, "transfer", -15, "May clinic rent"},
		{1, 450, "cash", -8, "Utilities settlement"},
		{2, 800, "card", -6, "Spring campaign"},
		{3, 180, "transfer", -2, "Medical waste pickup"},
	}
	for _, e := range expensePayments {
		if e.expenseIdx >= len(c.expenseIDs) {
			continue
		}
		at := timeAt(e.dayOff, 8, 30)
		expenseID := c.expenseIDs[e.expenseIdx]
		expenseBal := c.expenseBal[expenseID]
		if err := recordTransactionSource(ctx, tx, c, expenseBal, c.selfBalanceID, e.amount,
			"charge", "other", "expense", expenseID, e.desc, at); err != nil {
			return err
		}
		if err := recordTransactionSource(ctx, tx, c, c.selfBalanceID, expenseBal, e.amount,
			"payment", e.method, "expense", expenseID, e.desc, at); err != nil {
			return err
		}
	}
	return nil
}

func seedAppointmentsAndInvoices(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	if len(c.doctorIDs) == 0 {
		return nil
	}
	room1 := c.rooms["Room 1"]
	room2 := c.rooms["Room 2"]

	type apt struct {
		patientIdx             int
		room                   string
		dayOff, hour           int
		duration               int
		status                 string
		procedureName          string
		productNames           []string
		paymentAmount          float64
		paymentMethod          string
		applyOfferToBotox      bool
		notes, completionNotes string
	}
	appointments := []apt{
		// Past: completed, fully paid in cash.
		{0, room1, -7, 10, 60, "Completed", "Botox Full", nil, 220, "cash", false, "Routine touch-up", "Done; client happy"},
		// Past: completed, 20% Botox offer applied, paid by card.
		{3, room2, -5, 14, 60, "Completed", "Botox Migraine", nil, 320, "card", true, "Migraine session", ""},
		// Past: completed, partial payment ($150 of $250 lips filler).
		{2, room1, -3, 11, 45, "Completed", "Lips", nil, 150, "cash", false, "First filler", ""},
		{7, room2, -12, 13, 75, "Completed", "Full Body Package 2", nil, 250, "transfer", false, "Package session 1", "No reaction"},
		{6, room1, -10, 12, 60, "Completed", "Profhilo Face", []string{"Post Laser Repair Balm"}, 180, "card", false, "Hydration session", ""},
		{11, room1, -2, 9, 30, "Completed", "Consultation with Dr. Joe", nil, 0, "", false, "Surgical consult", "Discussed hospital estimate"},
		{8, room2, 0, 11, 90, "In-Progress", "Face + Plasma (1 session)", []string{"SPF 50 Mineral Cream"}, 200, "cash", false, "Morpheus8 session", ""},
		// Today: scheduled (no invoice).
		{4, room1, 0, 16, 60, "Scheduled", "", nil, 0, "", false, "Lip filler consult", ""},
		// Tomorrow: scheduled retail order (products only, charge invoice, no payment).
		{1, room2, 1, 12, 30, "Scheduled", "", []string{"Vitamin C Serum", "Hyaluronic Acid Cream"}, 0, "", false, "Retail pickup", ""},
		// +3 days: scheduled.
		{5, room1, 3, 15, 60, "Scheduled", "", nil, 0, "", false, "", ""},
		{9, room2, 4, 10, 45, "Scheduled", "Mole Removal", nil, 0, "", false, "Minor surgery booking", ""},
		{10, room1, -1, 17, 30, "Cancelled", "", nil, 0, "", false, "Patch test", "Client rescheduled"},
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
			items = append(items, invoiceItem{
				itemType: "procedure", itemID: procID, qty: 1,
				amount: price, finalAmount: price,
			})
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO appointment_procedures (id, patient_id, procedure_id, appointment_id, notes, created_at, updated_at)
				 VALUES (?,?,?,?,?,?,?)`,
				newID(), patientID, procID, aptID, "", c.now, c.now,
			); err != nil {
				return err
			}
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

		discountID := ""
		if a.applyOfferToBotox {
			discountID, _ = botoxOffer(ctx, tx)
		}
		invoiceID, total, err := createInvoice(ctx, tx, c, patientBal, items, discountID)
		if err != nil {
			return err
		}

		// Record the charge (no actual money moved).
		if err := recordTransactionSource(ctx, tx, c, c.selfBalanceID, patientBal, total,
			"charge", "other", "invoice", invoiceID, "Invoice charge", startStr); err != nil {
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
}

func createInvoice(ctx context.Context, tx *sql.Tx, c *demoCtx, patientBal string, items []invoiceItem, discountID string) (string, float64, error) {
	var nextNumber int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(invoice_number), 0) + 1 FROM invoices`,
	).Scan(&nextNumber); err != nil {
		return "", 0, err
	}

	var amount float64
	for _, it := range items {
		amount += it.amount * float64(it.qty)
	}

	var discountValue float64
	if discountID != "" {
		var discountType, valueType string
		var value float64
		if err := tx.QueryRowContext(ctx,
			`SELECT discount_type, value_type, value FROM discounts WHERE id = ?`,
			discountID,
		).Scan(&discountType, &valueType, &value); err != nil {
			return "", 0, err
		}
		if discountType != "offer" {
			return "", 0, fmt.Errorf("invoice discount %s is not an offer", discountID)
		}
		if valueType == "percentage" {
			discountValue = amount * value / 100
		} else {
			discountValue = value
		}
		if discountValue > amount {
			discountValue = amount
		}
	}
	finalAmount := amount - discountValue

	invID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, discount_id, discount_value, final_amount, currency_id, notes, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		invID, nextNumber, c.selfBalanceID, patientBal, amount, discountID, discountValue, finalAmount, c.currencyID, "", c.adminUserID, c.now, c.now,
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
		if it.itemType == "product" {
			if _, err := tx.ExecContext(ctx,
				`UPDATE products SET quantity = quantity - ? WHERE id = ?`,
				it.qty, it.itemID,
			); err != nil {
				return "", 0, err
			}
		}
	}
	return invID, finalAmount, nil
}

func createSupplierInvoice(ctx context.Context, tx *sql.Tx, c *demoCtx, supplierBal string, items []invoiceItem, at string) (string, float64, error) {
	var nextNumber int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(invoice_number), 0) + 1 FROM invoices`,
	).Scan(&nextNumber); err != nil {
		return "", 0, err
	}

	var amount float64
	for _, it := range items {
		amount += it.amount * float64(it.qty)
	}

	invID := newID()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, final_amount, currency_id, notes, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		invID, nextNumber, supplierBal, c.selfBalanceID, amount, amount, c.currencyID, "Supplier stock invoice", c.adminUserID, at, at,
	); err != nil {
		return "", 0, err
	}

	for _, it := range items {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO invoice_items (id, invoice_id, item_type, item_id, quantity, amount, final_amount, notes, created_at)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			newID(), invID, it.itemType, it.itemID, it.qty, it.amount, it.finalAmount, "", at,
		); err != nil {
			return "", 0, err
		}
		if it.itemType == "product" {
			if _, err := tx.ExecContext(ctx,
				`UPDATE products SET quantity = quantity + ? WHERE id = ?`,
				it.qty, it.itemID,
			); err != nil {
				return "", 0, err
			}
		}
	}
	return invID, amount, nil
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

// Notifications

func seedNotifications(ctx context.Context, tx *sql.Tx, c *demoCtx) error {
	notifications := []struct {
		title, description, action string
		read                       bool
	}{
		{"Low stock: Botox Vial 100u", "Quantity 8 has dropped near min threshold.", "/inventory", false},
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
