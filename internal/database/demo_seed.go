package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"clinic-api/internal/auth"

	"github.com/google/uuid"
)

// SeedDemo layers a rich demo dataset on top of the base seed (SeedIfEmpty).
// Idempotent: skips everything if employees already exist (taken as the
// signal that the demo has been applied).
func SeedDemo(db *sql.DB) error {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM employees").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		log.Println("Demo data already present; skipping demo seed")
		return nil
	}

	ctx, err := buildDemoContext(db)
	if err != nil {
		return fmt.Errorf("build demo context: %w", err)
	}

	steps := []struct {
		name string
		fn   func(*sql.DB, *demoCtx) error
	}{
		{"medicines", seedDemoMedicines},
		{"employees", seedDemoEmployees},
		{"schedules", seedDemoSchedules},
		{"supplier balances", seedDemoSupplierBalances},
		{"patients", seedDemoPatients},
		{"patient allergies & medicines", seedDemoPatientAllergiesAndMeds},
		{"prescriptions", seedDemoPrescriptions},
		{"discounts", seedDemoDiscounts},
		{"appointments & invoices", seedDemoAppointmentsAndInvoices},
		{"client payments", seedDemoClientPayments},
		{"supplier invoices", seedDemoSupplierInvoices},
		{"supplier payments", seedDemoSupplierPayments},
		{"employee payments", seedDemoEmployeePayments},
		{"bookings", seedDemoBookings},
		{"notifications", seedDemoNotifications},
	}
	for _, s := range steps {
		if err := s.fn(db, ctx); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	log.Println("Demo seed complete")
	return nil
}

// Context

type demoCtx struct {
	currencyID    string
	selfBalanceID string
	adminUserID   string

	procByName     map[string]string
	prodByName     map[string]string
	allergyByName  map[string]string
	roomByName     map[string]string
	supplierByName map[string]string
	cityByName     map[string]string
	countryByName  map[string]string

	medicineIDs []string
	employeeIDs []string
	doctorIDs   []string
	patientIDs  []string
}

func buildDemoContext(db *sql.DB) (*demoCtx, error) {
	c := &demoCtx{
		procByName:     map[string]string{},
		prodByName:     map[string]string{},
		allergyByName:  map[string]string{},
		roomByName:     map[string]string{},
		supplierByName: map[string]string{},
		cityByName:     map[string]string{},
		countryByName:  map[string]string{},
	}
	if err := db.QueryRow(`SELECT id FROM currencies WHERE code = 'USD'`).Scan(&c.currencyID); err != nil {
		return nil, fmt.Errorf("usd currency: %w", err)
	}
	if err := db.QueryRow(`SELECT id FROM balances WHERE entity_type = 'self'`).Scan(&c.selfBalanceID); err != nil {
		return nil, fmt.Errorf("self balance: %w", err)
	}
	if err := db.QueryRow(`SELECT id FROM users WHERE username = 'admin'`).Scan(&c.adminUserID); err != nil {
		return nil, fmt.Errorf("admin user: %w", err)
	}
	if err := fillNameMap(db, `SELECT id, name FROM procedures`, c.procByName); err != nil {
		return nil, err
	}
	if err := fillNameMap(db, `SELECT id, name FROM products`, c.prodByName); err != nil {
		return nil, err
	}
	if err := fillNameMap(db, `SELECT id, name FROM allergies`, c.allergyByName); err != nil {
		return nil, err
	}
	if err := fillNameMap(db, `SELECT id, name FROM rooms`, c.roomByName); err != nil {
		return nil, err
	}
	if err := fillNameMap(db, `SELECT id, name FROM suppliers`, c.supplierByName); err != nil {
		return nil, err
	}
	if err := fillNameMap(db, `SELECT id, name FROM lebanon_cities`, c.cityByName); err != nil {
		return nil, err
	}
	if err := fillNameMap(db, `SELECT id, name FROM countries`, c.countryByName); err != nil {
		return nil, err
	}
	return c, nil
}

func fillNameMap(db *sql.DB, query string, m map[string]string) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		if _, exists := m[name]; !exists {
			m[name] = id
		}
	}
	return rows.Err()
}

// Helpers

func rfc3339(t time.Time) string {
	return t.Format(time.RFC3339)
}

func dateOnly(t time.Time) string {
	return t.Format("2006-01-02")
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

// createBalance inserts an entity balance if missing. Returns the balance id.
func createBalance(db *sql.DB, entityType, entityID, entityName, currencyID string) (string, error) {
	now := rfc3339(time.Now())
	id := newID()
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, created_at, updated_at)
		 VALUES (?,?,?,?,?,0,?,?)`,
		id, entityType, entityID, entityName, currencyID, now, now,
	); err != nil {
		return "", err
	}
	var out string
	if err := db.QueryRow(
		`SELECT id FROM balances WHERE entity_type = ? AND entity_id = ? AND currency_id = ?`,
		entityType, entityID, currencyID,
	).Scan(&out); err != nil {
		return "", err
	}
	return out, nil
}

// recordTransaction inserts a balance_transaction and updates both balance
// amounts in one shot. Mirrors BalanceTransaction.CreateWithTx but without the
// model indirection so it can be used freely from seed functions.
func recordTransaction(db *sql.DB, fromID, toID string, amount float64, currencyID, txType, txMethod, description, createdBy, at string) error {
	if _, err := db.Exec(
		`INSERT INTO balance_transactions (id, from_balance_id, to_balance_id, amount, currency_id, transaction_type, transaction_method, description, created_by, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		newID(), fromID, toID, amount, currencyID, txType, txMethod, description, createdBy, at,
	); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE balances SET amount = amount - ?, updated_at = ? WHERE id = ?`, amount, at, fromID); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE balances SET amount = amount + ?, updated_at = ? WHERE id = ?`, amount, at, toID); err != nil {
		return err
	}
	return nil
}

// nextInvoiceNumber fetches MAX(invoice_number)+1.
func nextInvoiceNumber(db *sql.DB) (int, error) {
	var n int
	if err := db.QueryRow(`SELECT COALESCE(MAX(invoice_number), 0) + 1 FROM invoices`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// Medicines

func seedDemoMedicines(db *sql.DB, c *demoCtx) error {
	medicines := []struct{ name, desc string }{
		{"Amoxicillin 500mg", "Broad-spectrum antibiotic, oral capsules"},
		{"Cephalexin 500mg", "First-generation cephalosporin antibiotic"},
		{"Paracetamol 500mg", "Analgesic and antipyretic"},
		{"Ibuprofen 400mg", "NSAID for pain and inflammation"},
		{"Diclofenac 50mg", "NSAID, post-procedural pain"},
		{"Tramadol 50mg", "Centrally-acting analgesic"},
		{"Dexamethasone 4mg", "Corticosteroid for swelling"},
		{"Cetirizine 10mg", "Antihistamine, allergy relief"},
		{"Diphenhydramine 25mg", "Sedating antihistamine"},
		{"Omeprazole 20mg", "Proton pump inhibitor"},
		{"Mupirocin 2% Ointment", "Topical antibiotic for wounds"},
		{"Fusidic Acid 2% Cream", "Topical antibiotic"},
		{"Hydrocortisone 1% Cream", "Mild topical corticosteroid"},
		{"Arnica Gel", "Topical anti-bruising"},
		{"Vitamin C 1000mg", "Immune and skin support"},
		{"Minoxidil 5% Solution", "Topical hair-loss treatment"},
		{"Finasteride 1mg", "Oral hair-loss treatment"},
	}
	now := rfc3339(time.Now())
	for _, m := range medicines {
		id := newID()
		if _, err := db.Exec(
			`INSERT INTO medicines (id, name, description, created_at) VALUES (?,?,?,?)`,
			id, m.name, m.desc, now,
		); err != nil {
			return err
		}
		c.medicineIDs = append(c.medicineIDs, id)
	}
	log.Printf("Demo: seeded %d medicines", len(medicines))
	return nil
}

// Employees

type demoEmployee struct {
	firstName, lastName, role, contact, email, dob, employment string
	hasUser                                                    bool
	username, displayName, userRole                            string
	salary                                                     float64
	isDoctor                                                   bool
}

func seedDemoEmployees(db *sql.DB, c *demoCtx) error {
	employees := []demoEmployee{
		{"Julian", "Vance", "Plastic Surgeon", "+1 555 0107", "julian.vance@example.org", "1978-04-12", "Full-time",
			true, "jvance", "Dr. Julian Vance", "admin", 6000, true},
		{"Lana", "Haddad", "Dermatologist", "+1 555 0108", "lana.haddad@example.org", "1984-09-22", "Full-time",
			true, "lhaddad", "Dr. Lana Haddad", "admin", 5200, true},
		{"Samira", "Mansour", "Clinic Manager", "+1 555 0109", "samira.mansour@example.org", "1981-02-05", "Full-time",
			true, "smansour", "Samira Mansour", "admin", 2800, false},
		{"Maya", "Aoun", "Nurse", "+1 555 0113", "maya.aoun@example.org", "1990-07-17", "Full-time",
			true, "maoun", "Maya Aoun", "user", 1500, false},
		{"Rita", "Saad", "Nurse", "+1 555 0114", "rita.saad@example.org", "1992-11-30", "Full-time",
			false, "", "", "", 1400, false},
		{"Jana", "El-Khoury", "Aesthetician", "+1 555 0115", "jana.elkhoury@example.org", "1995-05-14", "Full-time",
			false, "", "", "", 1200, false},
		{"Tony", "Ghosn", "Laser Technician", "+1 555 0116", "tony.ghosn@example.org", "1988-03-08", "Full-time",
			false, "", "", "", 1600, false},
		{"Rami", "Najjar", "Receptionist", "+1 555 0117", "rami.najjar@example.org", "1996-12-03", "Part-time",
			false, "", "", "", 900, false},
	}

	now := rfc3339(time.Now())
	today := dateOnly(time.Now())

	for _, e := range employees {
		empID := newID()

		var userIDPtr any
		if e.hasUser {
			uid := newID()
			hash, err := auth.HashPassword("demo123")
			if err != nil {
				return err
			}
			if _, err := db.Exec(
				`INSERT INTO users (id, username, password_hash, display_name, role, is_active, created_at, updated_at)
				 VALUES (?,?,?,?,?,1,?,?)`,
				uid, e.username, hash, e.displayName, e.userRole, now, now,
			); err != nil {
				return err
			}
			userIDPtr = uid
		} else {
			userIDPtr = ""
		}

		if _, err := db.Exec(
			`INSERT INTO employees (id, user_id, first_name, last_name, role, contact, email, date_of_birth, employment_type, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			empID, userIDPtr, e.firstName, e.lastName, e.role, e.contact, e.email, e.dob, e.employment, now, now,
		); err != nil {
			return err
		}

		if _, err := createBalance(db, "employee", empID, e.firstName+" "+e.lastName, c.currencyID); err != nil {
			return err
		}

		if _, err := db.Exec(
			`INSERT INTO employee_salaries (id, employee_id, amount, currency_id, is_active, effective_date, notes, created_at, updated_at)
			 VALUES (?,?,?,?,1,?,?,?,?)`,
			newID(), empID, e.salary, c.currencyID, today, "Current base salary", now, now,
		); err != nil {
			return err
		}

		c.employeeIDs = append(c.employeeIDs, empID)
		if e.isDoctor {
			c.doctorIDs = append(c.doctorIDs, empID)
		}
	}
	log.Printf("Demo: seeded %d employees (%d doctors, %d user accounts)",
		len(employees), len(c.doctorIDs), 4)
	return nil
}

// Schedules

func seedDemoSchedules(db *sql.DB, c *demoCtx) error {
	// Mon–Fri 09:00-18:00 for the first 5 employees (doctors + manager + nurses).
	// Saturday 10:00-14:00 for doctors only.
	now := rfc3339(time.Now())
	today := dateOnly(time.Now())
	count := 0
	for i, empID := range c.employeeIDs {
		if i >= 5 {
			break
		}
		for day := 1; day <= 5; day++ {
			if _, err := db.Exec(
				`INSERT INTO schedule_availability (id, employee_id, day_of_week, start_time, end_time, effective_date, created_at)
				 VALUES (?,?,?,?,?,?,?)`,
				newID(), empID, day, "09:00", "18:00", today, now,
			); err != nil {
				return err
			}
			count++
		}
	}
	for _, empID := range c.doctorIDs {
		if _, err := db.Exec(
			`INSERT INTO schedule_availability (id, employee_id, day_of_week, start_time, end_time, effective_date, created_at)
			 VALUES (?,?,6,?,?,?,?)`,
			newID(), empID, "10:00", "14:00", today, now,
		); err != nil {
			return err
		}
		count++
	}
	log.Printf("Demo: seeded %d schedule rows", count)
	return nil
}

// Supplier balances

func seedDemoSupplierBalances(db *sql.DB, c *demoCtx) error {
	type sup struct{ id, name string }
	rows, err := db.Query(`SELECT id, name FROM suppliers`)
	if err != nil {
		return err
	}
	var suppliers []sup
	for rows.Next() {
		var s sup
		if err := rows.Scan(&s.id, &s.name); err != nil {
			continue
		}
		suppliers = append(suppliers, s)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return rowsErr
	}

	for _, s := range suppliers {
		if _, err := createBalance(db, "supplier", s.id, s.name, c.currencyID); err != nil {
			return err
		}
	}
	log.Printf("Demo: ensured %d supplier balances", len(suppliers))
	return nil
}

// Patients

type demoPatient struct {
	first, middle, last, gender, dob, contact, email string
	emName, emPhone                                  string
	weight, height                                   float64
	blood, country, city, address                    string
	referralSource, notes                            string
}

func seedDemoPatients(db *sql.DB, c *demoCtx) error {
	patients := []demoPatient{
		{"Nour", "", "Abou Khalil", "Female", "1991-03-14", "+1 555 0118", "nour.ak@example.com",
			"Layla Abou Khalil", "+1 555 0119", 58, 165, "A+", "Lebanon", "Beirut", "Achrafieh, Rue Sursock",
			"Instagram", "Regular client; prefers late-afternoon slots"},
		{"Rami", "", "Chahine", "Male", "1985-08-22", "+1 555 0120", "rami.c@example.com",
			"Mona Chahine", "+1 555 0133", 82, 180, "O+", "Lebanon", "Jounieh", "Kaslik, Main Road",
			"Google", "Prefers Dr. Julian"},
		{"Lea", "Marie", "Sassine", "Female", "1994-12-01", "+1 555 0134", "lea.sassine@example.com",
			"Paul Sassine", "+1 555 0135", 54, 168, "B+", "Lebanon", "Beirut", "Gemmayzeh",
			"Friend referral", ""},
		{"Karim", "", "Fakhoury", "Male", "1979-05-19", "+1 555 0136", "karim.f@example.com",
			"Sandra Fakhoury", "+1 555 0137", 90, 183, "A-", "Lebanon", "Baabda", "Rabieh, Block 4",
			"Walk-in", "Has recurrent migraine; botox for migraine"},
		{"Yasmina", "", "Daou", "Female", "1996-02-28", "+1 555 0138", "yasmina.daou@example.com",
			"Rima Daou", "+1 555 0139", 60, 170, "AB+", "Lebanon", "Tripoli", "Dam wel Farez",
			"Instagram", "First-time lip filler"},
		{"Elie", "", "Zogheib", "Male", "1982-07-09", "+1 555 0140", "elie.z@example.com",
			"Maya Zogheib", "+1 555 0141", 78, 178, "O-", "Lebanon", "Jbeil", "Old Souk",
			"TikTok", ""},
		{"Tala", "", "Nasrallah", "Female", "1999-10-15", "+1 555 0142", "tala.n@example.com",
			"Omar Nasrallah", "+1 555 0143", 52, 162, "B-", "Lebanon", "Beirut", "Verdun, Rue Makdessi",
			"Instagram", "Interested in full-body laser package"},
		{"Fadi", "", "Karam", "Male", "1970-11-25", "+1 555 0144", "fadi.k@example.com",
			"Nadine Karam", "+1 555 0145", 95, 175, "A+", "Lebanon", "Sidon", "Sea Road",
			"Google", "Follow-up rhinoplasty consultation"},
		{"Celine", "", "Abi Rached", "Female", "1987-06-06", "+1 555 0146", "celine.ar@example.com",
			"Georges Abi Rached", "+1 555 0147", 64, 170, "O+", "Lebanon", "Beirut", "Hamra, Makhoul St.",
			"Friend referral", "Prefers Dr. Lana"},
		{"Ziad", "", "Harb", "Male", "1993-01-21", "+1 555 0148", "ziad.h@example.com",
			"Carole Harb", "+1 555 0149", 74, 176, "A+", "Lebanon", "Zahle", "Mar Elias",
			"Instagram", ""},
		{"Rana", "", "El Hage", "Female", "1989-04-04", "+1 555 0150", "rana.eh@example.com",
			"Charbel El Hage", "+1 555 0151", 55, 163, "B+", "Lebanon", "Baabda", "Hazmieh",
			"Google", "Regular Morpheus8 sessions"},
		{"Marc", "", "Tabet", "Male", "1975-09-12", "+1 555 0152", "marc.t@example.com",
			"Joelle Tabet", "+1 555 0153", 88, 181, "AB-", "Lebanon", "Beirut", "Ashrafieh, Sodeco",
			"Walk-in", "Allergic to lidocaine — use alternative"},
		{"Dina", "", "Mouawad", "Female", "1983-08-30", "+1 555 0154", "dina.m@example.com",
			"Samir Mouawad", "+1 555 0155", 66, 169, "O+", "Lebanon", "Batroun", "Old Souk",
			"Instagram", ""},
		{"Georges", "", "Bou Younes", "Male", "1968-12-19", "+1 555 0156", "georges.by@example.com",
			"Jacqueline Bou Younes", "+1 555 0157", 85, 174, "A+", "Lebanon", "Jounieh", "Sahel Alma",
			"Friend referral", "Hair restoration interest"},
		{"Maya", "", "Chalhoub", "Female", "1997-03-07", "+1 555 0158", "maya.c@example.com",
			"Rosy Chalhoub", "+1 555 0159", 57, 167, "B+", "Lebanon", "Metn", "Dbayeh",
			"TikTok", ""},
		{"Hadi", "", "Khalil", "Male", "1990-10-28", "+1 555 0160", "hadi.k@example.com",
			"Rania Khalil", "+1 555 0161", 79, 179, "O+", "Lebanon", "Beirut", "Mar Mikhael",
			"Google", ""},
		{"Sarah", "", "Moukarzel", "Female", "1992-05-16", "+1 555 0162", "sarah.m@example.com",
			"Eliane Moukarzel", "+1 555 0163", 61, 166, "A+", "Lebanon", "Kesrouan", "Zouk Mosbeh",
			"Instagram", "Penicillin allergy — no amoxicillin"},
		{"Anthony", "", "Semaan", "Male", "1980-02-11", "+1 555 0164", "anthony.s@example.com",
			"Christine Semaan", "+1 555 0165", 83, 182, "O+", "Lebanon", "Beirut", "Saifi Village",
			"Walk-in", ""},
		{"Layla", "", "Rizk", "Female", "1998-07-23", "+1 555 0166", "layla.r@example.com",
			"Sana Rizk", "+1 555 0167", 53, 164, "AB+", "Lebanon", "Aley", "Upper Aley",
			"Instagram", ""},
		{"Marwan", "", "Abboud", "Male", "1986-11-02", "+1 555 0168", "marwan.a@example.com",
			"Hala Abboud", "+1 555 0169", 80, 177, "A-", "Lebanon", "Baabda", "Yarze",
			"TikTok", ""},
		{"Carla", "", "Geagea", "Female", "1995-04-18", "+1 555 0170", "carla.g@example.com",
			"Roula Geagea", "+1 555 0171", 59, 170, "B+", "Lebanon", "Bsharre", "Downtown",
			"Google", ""},
		{"Omar", "", "Bitar", "Male", "1973-06-30", "+1 555 0172", "omar.b@example.com",
			"Dana Bitar", "+1 555 0173", 92, 180, "O-", "Lebanon", "Beirut", "Koraytem",
			"Friend referral", "Considering gynecomastia surgery"},
		{"Roula", "", "Matta", "Female", "1988-09-09", "+1 555 0174", "roula.m@example.com",
			"Karim Matta", "+1 555 0175", 63, 168, "A+", "Lebanon", "Metn", "Broumana",
			"Instagram", ""},
		{"Peter", "", "Azar", "Male", "1984-01-05", "+1 555 0176", "peter.a@example.com",
			"Marianne Azar", "+1 555 0177", 86, 183, "O+", "Lebanon", "Jbeil", "Amchit",
			"Walk-in", ""},
		{"Noor", "", "Hamdan", "Female", "2000-12-25", "+1 555 0178", "noor.h@example.com",
			"Samar Hamdan", "+1 555 0179", 51, 161, "B+", "Lebanon", "Beirut", "Clemenceau",
			"TikTok", "Young patient; consent forms required"},
	}

	now := rfc3339(time.Now())

	for _, p := range patients {
		id := newID()
		countryID := c.countryByName[p.country]
		cityID := c.cityByName[p.city]
		if _, err := db.Exec(
			`INSERT INTO patients (id, first_name, middle_name, last_name, gender, date_of_birth,
				contact, email, emergency_contact_name, emergency_contact_phone,
				weight, height, blood_type, country_id, city_id, address, referral_id, referral_source, notes,
				created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,?,?,?,?)`,
			id, p.first, p.middle, p.last, p.gender, p.dob,
			p.contact, p.email, p.emName, p.emPhone,
			p.weight, p.height, p.blood, countryID, cityID, p.address, p.referralSource, p.notes,
			now, now,
		); err != nil {
			return err
		}
		if _, err := createBalance(db, "patient", id, p.first+" "+p.last, c.currencyID); err != nil {
			return err
		}
		c.patientIDs = append(c.patientIDs, id)
	}
	log.Printf("Demo: seeded %d patients", len(patients))
	return nil
}

// Patient allergies & medicines

func seedDemoPatientAllergiesAndMeds(db *sql.DB, c *demoCtx) error {
	// Map each patient index to a list of allergy names. Pulls from the names
	// seeded by seedAllergies; missing ones are silently skipped.
	allergyAssignments := map[int][]string{
		0:  {"Penicillin", "Latex"},
		1:  {"Pollen", "Dust Mites"},
		2:  {"Peanuts"},
		3:  {"Codeine", "Aspirin"},
		4:  {"Nickel", "Fragrance Mix"},
		6:  {"Shellfish", "Iodine Contrast Dye"},
		8:  {"Sulfonamides"},
		10: {"Pet Dander (Cat)", "Dust Mites"},
		11: {"Lidocaine", "Novocaine"},
		13: {"Ibuprofen"},
		14: {"Latex"},
		16: {"Penicillin", "Amoxicillin"},
		18: {"Pollen"},
		19: {"Hair Dye (PPD)", "Nickel"},
		21: {"Tramadol"},
		23: {"Eggs", "Milk"},
	}

	now := rfc3339(time.Now())
	allergyCount := 0
	for idx, names := range allergyAssignments {
		if idx >= len(c.patientIDs) {
			continue
		}
		for _, name := range names {
			allergyID, ok := c.allergyByName[name]
			if !ok {
				continue
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO patient_allergies (id, patient_id, allergy_id, notes, created_at)
				 VALUES (?,?,?,?,?)`,
				newID(), c.patientIDs[idx], allergyID, "", now,
			); err != nil {
				return err
			}
			allergyCount++
		}
	}

	// Patient active medicines: a few patients on standing prescriptions.
	medAssignments := map[int][]string{
		3:  {"Omeprazole 20mg"},
		10: {"Cetirizine 10mg"},
		13: {"Paracetamol 500mg"},
		16: {"Cephalexin 500mg"},
		21: {"Vitamin C 1000mg"},
	}
	medByName := map[string]string{}
	rows, err := db.Query(`SELECT id, name FROM medicines`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			medByName[name] = id
		}
	}
	rows.Close()

	medCount := 0
	for idx, names := range medAssignments {
		if idx >= len(c.patientIDs) {
			continue
		}
		for _, name := range names {
			medID, ok := medByName[name]
			if !ok {
				continue
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO patient_medicines (id, patient_id, medicine_id, is_active, notes, created_at)
				 VALUES (?,?,?,1,?,?)`,
				newID(), c.patientIDs[idx], medID, "Ongoing", now,
			); err != nil {
				return err
			}
			medCount++
		}
	}

	log.Printf("Demo: seeded %d patient allergies, %d active patient medicines", allergyCount, medCount)
	return nil
}

// Prescriptions

func seedDemoPrescriptions(db *sql.DB, c *demoCtx) error {
	if len(c.doctorIDs) == 0 {
		return nil
	}

	medByName := map[string]string{}
	rows, err := db.Query(`SELECT id, name FROM medicines`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err == nil {
			medByName[name] = id
		}
	}
	rows.Close()

	type presc struct {
		patientIdx int
		doctorIdx  int
		startOff   int
		endOff     int
		meds       []struct{ name, instructions, status string }
	}

	prescriptions := []presc{
		{3, 0, -14, -7, []struct{ name, instructions, status string }{
			{"Paracetamol 500mg", "1 tab every 6h as needed for headache", "completed"},
			{"Ibuprofen 400mg", "1 tab every 8h with food for 5 days", "completed"},
		}},
		{0, 1, -10, -3, []struct{ name, instructions, status string }{
			{"Mupirocin 2% Ointment", "Apply thin layer to injection sites twice daily for 5 days", "completed"},
			{"Arnica Gel", "Apply 3-4 times daily to bruising", "completed"},
		}},
		{11, 1, -4, 3, []struct{ name, instructions, status string }{
			{"Cephalexin 500mg", "1 cap three times daily for 7 days", "active"},
			{"Diclofenac 50mg", "1 tab twice daily for 3 days", "active"},
		}},
		{6, 0, 0, 6, []struct{ name, instructions, status string }{
			{"Arnica Gel", "Apply 3-4 times daily to treated area", "active"},
		}},
		{13, 1, -2, 5, []struct{ name, instructions, status string }{
			{"Finasteride 1mg", "1 tablet daily in the morning", "active"},
			{"Minoxidil 5% Solution", "1 ml to scalp twice daily", "active"},
		}},
		{17, 0, -1, 0, []struct{ name, instructions, status string }{
			{"Dexamethasone 4mg", "1 tab daily for 3 days after procedure", "active"},
		}},
	}

	now := rfc3339(time.Now())
	count := 0
	for _, p := range prescriptions {
		if p.patientIdx >= len(c.patientIDs) || p.doctorIdx >= len(c.doctorIDs) {
			continue
		}
		start := dateOnly(time.Now().AddDate(0, 0, p.startOff))
		var end string
		if p.endOff != 0 || p.startOff < p.endOff {
			end = dateOnly(time.Now().AddDate(0, 0, p.endOff))
		}

		pid := newID()
		if _, err := db.Exec(
			`INSERT INTO prescriptions (id, patient_id, prescribed_by_id, start_date, end_date, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?)`,
			pid, c.patientIDs[p.patientIdx], c.doctorIDs[p.doctorIdx], start, end, now, now,
		); err != nil {
			return err
		}
		for _, m := range p.meds {
			medID, ok := medByName[m.name]
			if !ok {
				continue
			}
			if _, err := db.Exec(
				`INSERT INTO prescription_medicines (id, medicine_id, prescription_id, instructions, status, created_at)
				 VALUES (?,?,?,?,?,?)`,
				newID(), medID, pid, m.instructions, m.status, now,
			); err != nil {
				return err
			}
		}
		count++
	}
	log.Printf("Demo: seeded %d prescriptions", count)
	return nil
}

// Discounts

func seedDemoDiscounts(db *sql.DB, c *demoCtx) error {
	now := rfc3339(time.Now())
	startOffer := dateOnly(time.Now().AddDate(0, 0, -15))
	endOffer := dateOnly(time.Now().AddDate(0, 0, 30))

	// 1) Offer: 20% off on any Botox procedure
	offerID := newID()
	if _, err := db.Exec(
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, max_usages, current_usages, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,NULL,0,?,?,1,?,?)`,
		offerID, "Spring Botox -20%", "20% off any Botox procedure through the promotion window",
		"offer", "percentage", 20.0, startOffer, endOffer, now, now,
	); err != nil {
		return err
	}
	botoxNames := []string{
		"Botox Full", "Botox Full (Dysport)", "Botox Full + Bunny Lines",
		"Botox Around Eyes", "Botox Frown Lines", "Botox Neck",
	}
	seen := map[string]bool{}
	for _, name := range botoxNames {
		// procByName collapses duplicates; link once per name to both gender rows.
		rows, err := db.Query(`SELECT id FROM procedures WHERE name = ?`, name)
		if err != nil {
			return err
		}
		var pids []string
		for rows.Next() {
			var pid string
			if err := rows.Scan(&pid); err != nil {
				continue
			}
			pids = append(pids, pid)
		}
		rows.Close()

		for _, pid := range pids {
			if seen[pid] {
				continue
			}
			seen[pid] = true
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO discount_items (id, discount_id, item_type, item_id, created_at) VALUES (?,?,?,?,?)`,
				newID(), offerID, "procedure", pid, now,
			); err != nil {
				return err
			}
		}
	}

	// 2) Voucher: $50 off skin booster procedures (10 codes)
	voucherID := newID()
	maxUses := 10
	if _, err := db.Exec(
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, max_usages, current_usages, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,0,?,?,1,?,?)`,
		voucherID, "Booster Voucher $50", "Redeem for $50 off any skin booster session",
		"voucher", "fixed", 50.0, maxUses, startOffer, endOffer, now, now,
	); err != nil {
		return err
	}
	boosterNames := []string{
		"Profhilo Face", "Profhilo Neck", "Profhilo Body",
		"Skinvive", "Jalupro Face", "Jalupro Eye",
	}
	for _, name := range boosterNames {
		if pid, ok := c.procByName[name]; ok {
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO discount_items (id, discount_id, item_type, item_id, created_at) VALUES (?,?,?,?,?)`,
				newID(), voucherID, "procedure", pid, now,
			); err != nil {
				return err
			}
		}
	}
	codes := []string{"BOOST-A1B2", "BOOST-C3D4", "BOOST-E5F6", "BOOST-G7H8", "BOOST-J9K0",
		"BOOST-L1M2", "BOOST-N3P4", "BOOST-Q5R6", "BOOST-S7T8", "BOOST-U9V0"}
	for i, code := range codes {
		isUsed := 0
		if i < 2 {
			isUsed = 1 // first two already redeemed
		}
		if _, err := db.Exec(
			`INSERT INTO vouchers (id, discount_id, code, is_used, created_at) VALUES (?,?,?,?,?)`,
			newID(), voucherID, code, isUsed, now,
		); err != nil {
			return err
		}
	}

	// 3) Gift: 15% off any cream product, a loyalty gift for regulars
	giftID := newID()
	if _, err := db.Exec(
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, max_usages, current_usages, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,NULL,0,NULL,NULL,1,?,?)`,
		giftID, "Loyalty Gift -15%", "15% off any cream product — for returning clients",
		"gift", "percentage", 15.0, now, now,
	); err != nil {
		return err
	}
	creamNames := []string{
		"Sodermix Cream", "Keloplast Cream", "Boost Lift Cream", "Boost Relax Cream",
		"Boost C Serum", "Boost Glow Serum", "Retinol Serum 0.5%", "Hair Care Serum",
	}
	for _, name := range creamNames {
		if pid, ok := c.prodByName[name]; ok {
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO discount_items (id, discount_id, item_type, item_id, created_at) VALUES (?,?,?,?,?)`,
				newID(), giftID, "product", pid, now,
			); err != nil {
				return err
			}
		}
	}

	// 4) Offer: flat $30 off laser hair removal singles
	laserID := newID()
	if _, err := db.Exec(
		`INSERT INTO discounts (id, name, description, discount_type, value_type, value, max_usages, current_usages, start_date, end_date, is_active, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,NULL,0,?,?,1,?,?)`,
		laserID, "Summer Laser -$30", "$30 off single-area laser hair removal",
		"offer", "fixed", 30.0, startOffer, endOffer, now, now,
	); err != nil {
		return err
	}
	laserNames := []string{"Full Face", "Underarms", "Full Arms", "Full Legs", "Brazilian", "Chest", "Full Back"}
	for _, name := range laserNames {
		rows, err := db.Query(
			`SELECT p.id FROM procedures p
			 JOIN procedure_categories c ON c.id = p.category_id
			 JOIN procedure_categories pc ON pc.id = c.parent_id
			 WHERE p.name = ? AND pc.name = 'Laser Hair Removal'`, name)
		if err != nil {
			return err
		}
		var pids []string
		for rows.Next() {
			var pid string
			if err := rows.Scan(&pid); err == nil {
				pids = append(pids, pid)
			}
		}
		rows.Close()

		for _, pid := range pids {
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO discount_items (id, discount_id, item_type, item_id, created_at) VALUES (?,?,?,?,?)`,
				newID(), laserID, "procedure", pid, now,
			); err != nil {
				return err
			}
		}
	}

	log.Printf("Demo: seeded 4 discounts (offer + voucher + gift + laser offer) with %d voucher codes", len(codes))
	return nil
}

// Appointments, patient procedures, invoices

// demoAppt is a compact blueprint for a seeded appointment. startOff is in
// days relative to today (negative = past, 0 = today, positive = future).
type demoAppt struct {
	patientIdx int
	procName   string
	room       string
	startOff   int
	hour       int
	minute     int
	durMin     int
	status     string // "Completed", "Scheduled", "In-Progress", "Cancelled"
	productAdd string // optional product name to add to invoice (for Completed only)
	notes      string
}

func seedDemoAppointmentsAndInvoices(db *sql.DB, c *demoCtx) error {
	appts := []demoAppt{
		// Past 30 days: completed
		{0, "Botox Full", "Room 1", -30, 10, 0, 45, "Completed", "", "Touch-up at week 2 if needed"},
		{2, "Lips", "Room 2", -28, 11, 0, 60, "Completed", "Arnica Gel", "1ml Juvederm Volbella"},
		{4, "Lips", "Room 2", -27, 14, 0, 60, "Completed", "", "First-time filler, 0.5ml"},
		{1, "Full Face", "Room 5", -26, 15, 30, 30, "Completed", "", "Laser hair removal session 1"},
		{7, "Consultation with Dr. Joe", "Room 4", -25, 9, 0, 30, "Completed", "", "Pre-rhinoplasty consult"},
		{3, "Botox Migraine", "Room 1", -24, 13, 0, 45, "Completed", "", "Migraine protocol — 30 units"},
		{10, "Face + Plasma", "Room 3", -21, 10, 30, 90, "Completed", "Keloplast Cream", "Morpheus8 session 1/3"},
		{5, "Botox Full", "Room 1", -20, 11, 0, 45, "Completed", "", ""},
		{9, "Full Legs", "Room 5", -19, 16, 0, 60, "Completed", "", "Laser session 1"},
		{6, "Carbon Peel", "Room 2", -18, 14, 30, 45, "Completed", "", ""},
		{12, "Profhilo Face", "Room 3", -17, 10, 0, 45, "Completed", "", "Session 1 of 2"},
		{14, "Underarms", "Room 5", -16, 13, 0, 20, "Completed", "", ""},
		{16, "Cheeks", "Room 2", -15, 11, 30, 75, "Completed", "", "2ml Juvederm Voluma XC"},
		{8, "Jalupro Face", "Room 3", -14, 9, 30, 45, "Completed", "", ""},
		{19, "Botox Frown Lines", "Room 1", -13, 15, 0, 30, "Completed", "", ""},
		{0, "Skinvive", "Room 3", -12, 14, 0, 45, "Completed", "", "Session 2 for Nour"},
		{11, "Sodermix", "Room 4", -11, 10, 0, 15, "Completed", "Sodermix Cream", "Scar cream pickup"},
		{17, "Botox Full", "Room 1", -10, 13, 0, 45, "Completed", "", ""},
		{20, "Nasolabial Folds", "Room 2", -9, 11, 0, 60, "Completed", "", "1ml Juvederm Ultra XC"},
		{22, "Abdomen", "Room 5", -8, 16, 0, 45, "Completed", "", ""},
		{13, "PRP Hair Restoration", "Room 3", -7, 10, 30, 75, "Completed", "", "Session 1 of 4"},
		{2, "Botox Around Eyes", "Room 1", -6, 14, 0, 30, "Completed", "", ""},
		{15, "Full Face", "Room 5", -5, 13, 30, 30, "Completed", "", "Laser session 2"},
		{4, "Scalpel Blade #11 (10-pack)", "Room 4", -5, 10, 0, 15, "Cancelled", "", "Patient cancelled, rescheduled"},
		{23, "Full Body Package 1", "Room 6", -4, 11, 0, 90, "Completed", "", "Package session 1"},

		// This week: mix of statuses
		{1, "Botox Full (Dysport)", "Room 1", -3, 10, 0, 45, "Completed", "", ""},
		{10, "Face + Plasma", "Room 3", -2, 11, 30, 90, "Completed", "", "Morpheus8 session 2/3"},
		{6, "Melasma Full Face", "Room 2", -1, 14, 0, 45, "Completed", "", ""},
		{5, "Rhinoplasty", "Hospital", -1, 8, 0, 180, "Completed", "", "Surgery completed successfully"},
		{18, "Brazilian", "Room 5", 0, 9, 0, 30, "In-Progress", "", "Morning laser slot"},
		{7, "Consultation with Dr. Joe", "Room 4", 0, 10, 0, 30, "Scheduled", "", "Follow-up post-op check"},
		{12, "Profhilo Face", "Room 3", 0, 11, 30, 45, "Scheduled", "", "Session 2 of 2"},
		{0, "Botox Around Eyes", "Room 1", 0, 13, 0, 30, "Scheduled", "", ""},
		{9, "Full Legs", "Room 5", 0, 14, 0, 60, "Scheduled", "", "Laser session 2"},
		{16, "Jawline", "Room 2", 0, 15, 30, 60, "Scheduled", "", ""},

		// Next 14 days: scheduled
		{3, "Botox Migraine", "Room 1", 1, 10, 0, 45, "Scheduled", "", "Next migraine dose"},
		{4, "Lips", "Room 2", 1, 11, 30, 60, "Scheduled", "", "Top-up after 2 weeks"},
		{13, "PRP Hair Restoration", "Room 3", 2, 10, 30, 75, "Scheduled", "", "Session 2 of 4"},
		{21, "Consultation with Dr. Joe", "Room 4", 2, 14, 0, 30, "Scheduled", "", "Gynecomastia consult"},
		{14, "Full Arms", "Room 5", 3, 9, 30, 45, "Scheduled", "", ""},
		{8, "Jalupro Face", "Room 3", 3, 11, 0, 45, "Scheduled", "", "Session 2"},
		{17, "Cheeks", "Room 2", 4, 13, 0, 75, "Scheduled", "", ""},
		{15, "Full Face", "Room 5", 4, 15, 0, 30, "Scheduled", "", "Laser session 3"},
		{22, "Chest", "Room 5", 5, 10, 0, 30, "Scheduled", "", ""},
		{1, "Face & Neck + Plasma", "Room 3", 6, 10, 0, 120, "Scheduled", "", "Morpheus8 combo"},
		{10, "Face + Plasma", "Room 3", 7, 11, 0, 90, "Scheduled", "", "Morpheus8 session 3/3"},
		{24, "Full Face", "Room 5", 8, 13, 30, 30, "Scheduled", "", "First laser session"},
		{6, "Nasolabial Folds", "Room 2", 9, 14, 0, 60, "Scheduled", "", ""},
		{11, "Filler Dissolver", "Room 2", 10, 11, 0, 30, "Scheduled", "", "Dissolve old lip filler"},
		{19, "Botox Full", "Room 1", 11, 10, 0, 45, "Scheduled", "", ""},
		{20, "Marionette Lines", "Room 2", 12, 15, 0, 45, "Scheduled", "", ""},
		{0, "Profhilo Face", "Room 3", 13, 10, 30, 45, "Scheduled", "", "Next booster"},
		{2, "Lips", "Room 2", 14, 11, 0, 60, "Scheduled", "", ""},
	}

	// Pre-index procedures by name (first match).
	// For Botox-named procedures, pick whichever gender row exists first.
	lookupProc := func(name string) string {
		return c.procByName[name]
	}

	// Cache patient balances for invoice creation
	patientBal := map[string]string{}
	for _, pid := range c.patientIDs {
		var bid string
		if err := db.QueryRow(
			`SELECT id FROM balances WHERE entity_type = 'patient' AND entity_id = ? AND currency_id = ?`,
			pid, c.currencyID,
		).Scan(&bid); err == nil {
			patientBal[pid] = bid
		}
	}

	createdAppts := 0
	createdInvoices := 0

	for i, a := range appts {
		if a.patientIdx >= len(c.patientIDs) {
			continue
		}
		patientID := c.patientIDs[a.patientIdx]
		roomID, ok := c.roomByName[a.room]
		if !ok {
			continue
		}

		start := time.Now().AddDate(0, 0, a.startOff)
		start = time.Date(start.Year(), start.Month(), start.Day(), a.hour, a.minute, 0, 0, start.Location())
		end := start.Add(time.Duration(a.durMin) * time.Minute)

		appointmentID := newID()
		now := rfc3339(time.Now())

		cancelNotes := ""
		completionNotes := ""
		if a.status == "Cancelled" {
			cancelNotes = "Cancelled by patient"
		}
		if a.status == "Completed" {
			completionNotes = "Procedure completed without complications"
		}

		if _, err := db.Exec(
			`INSERT INTO appointments (id, patient_id, room_id, start_time, end_time, status,
				notes, cancel_notes, completion_notes, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			appointmentID, patientID, roomID,
			rfc3339(start), rfc3339(end), a.status,
			a.notes, cancelNotes, completionNotes, now, now,
		); err != nil {
			return fmt.Errorf("insert appointment #%d: %w", i, err)
		}
		createdAppts++

		// Link a patient_procedure to this appointment if the procedure resolves.
		procID := lookupProc(a.procName)
		if procID == "" {
			continue
		}
		ppStatus := "planned"
		switch a.status {
		case "Completed":
			ppStatus = "completed"
		case "In-Progress":
			ppStatus = "in-progress"
		}
		ppID := newID()
		appointmentIDPtr := appointmentID
		if _, err := db.Exec(
			`INSERT INTO patient_procedures (id, patient_id, procedure_id, appointment_id, status, notes, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?)`,
			ppID, patientID, procID, appointmentIDPtr, ppStatus, a.notes, now, now,
		); err != nil {
			return fmt.Errorf("insert patient_procedure: %w", err)
		}

		// Invoice only for Completed non-consultation appointments with a real price.
		if a.status != "Completed" {
			continue
		}
		var price float64
		if err := db.QueryRow(`SELECT price FROM procedures WHERE id = ?`, procID).Scan(&price); err != nil {
			continue
		}
		if price <= 0 {
			continue
		}
		fromBal := c.selfBalanceID
		toBal, ok := patientBal[patientID]
		if !ok {
			continue
		}

		if err := createClientInvoice(db, c, fromBal, toBal, a.procName, procID, price, a.productAdd, start); err != nil {
			return err
		}
		createdInvoices++
	}

	log.Printf("Demo: seeded %d appointments (%d with invoices)", createdAppts, createdInvoices)
	return nil
}

// createClientInvoice creates a client invoice (self -> patient) with the
// given procedure and an optional product line. Adjusts product stock,
// inserts balance transaction, and keeps invoice totals consistent.
func createClientInvoice(db *sql.DB, c *demoCtx, fromBal, toBal, procName, procID string, procPrice float64, productName string, at time.Time) error {
	invoiceID := newID()
	num, err := nextInvoiceNumber(db)
	if err != nil {
		return err
	}
	ts := rfc3339(at)

	lines := []invoiceLine{{itemType: "procedure", itemID: procID, quantity: 1, amount: procPrice, finalAmt: procPrice}}

	// Optionally attach a product line (e.g., take-home cream after procedure).
	if productName != "" {
		if pid, ok := c.prodByName[productName]; ok {
			var uPrice float64
			if err := db.QueryRow(`SELECT unit_price FROM products WHERE id = ?`, pid).Scan(&uPrice); err == nil && uPrice > 0 {
				lines = append(lines, invoiceLine{itemType: "product", itemID: pid, quantity: 1, amount: uPrice, finalAmt: uPrice})
			}
		}
	}

	// Apply a discount to some procedure lines probabilistically by name match.
	// ~1 in 4 completed procedures gets a discount of the right type.
	hasDiscount := false
	if strings.HasPrefix(procName, "Botox") && num%4 == 0 {
		if did, value, valueType := findDiscountByName(db, "Spring Botox -20%"); did != "" {
			applyDiscountToLine(&lines[0], value, valueType)
			lines[0].discounts = append(lines[0].discounts, did)
			hasDiscount = true
		}
	} else if (strings.Contains(procName, "Profhilo") || strings.Contains(procName, "Jalupro") || procName == "Skinvive") && num%3 == 0 {
		if did, value, valueType := findDiscountByName(db, "Booster Voucher $50"); did != "" {
			applyDiscountToLine(&lines[0], value, valueType)
			lines[0].discounts = append(lines[0].discounts, did)
			hasDiscount = true
		}
	}

	// Totals
	var amount, finalAmount float64
	for _, ln := range lines {
		amount += ln.amount
		finalAmount += ln.finalAmt
	}

	if _, err := db.Exec(
		`INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, final_amount, currency_id, notes, created_by, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		invoiceID, num, fromBal, toBal, amount, finalAmount, c.currencyID, "", "admin", ts, ts,
	); err != nil {
		return err
	}

	// Insert line items + discounts, adjust product stock.
	for _, ln := range lines {
		itemID := newID()
		if _, err := db.Exec(
			`INSERT INTO invoice_items (id, invoice_id, item_type, item_id, quantity, amount, final_amount, notes, created_at)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			itemID, invoiceID, ln.itemType, ln.itemID, ln.quantity, ln.amount, ln.finalAmt, "", ts,
		); err != nil {
			return err
		}
		for _, did := range ln.discounts {
			discountValue := ln.amount - ln.finalAmt
			if _, err := db.Exec(
				`INSERT INTO invoice_item_discounts (id, invoice_item_id, discount_id, discount_value, created_at)
				 VALUES (?,?,?,?,?)`,
				newID(), itemID, did, discountValue, ts,
			); err != nil {
				return err
			}
			if _, err := db.Exec(
				`UPDATE discounts SET current_usages = current_usages + 1, updated_at = ? WHERE id = ?`, ts, did,
			); err != nil {
				return err
			}
		}
		if ln.itemType == "product" {
			// Client invoice has from=self, so stock decreases.
			if _, err := db.Exec(
				`UPDATE products SET quantity = quantity - ? WHERE id = ?`, ln.quantity, ln.itemID,
			); err != nil {
				return err
			}
		}
	}

	// Balance transaction for the charge (self to patient).
	method := "cash"
	if hasDiscount {
		method = "discount"
	}
	if err := recordTransaction(db, fromBal, toBal, finalAmount, c.currencyID, "charge", method,
		fmt.Sprintf("Invoice #%d", num), "admin", ts); err != nil {
		return err
	}
	return nil
}

func findDiscountByName(db *sql.DB, name string) (id string, value float64, valueType string) {
	_ = db.QueryRow(`SELECT id, value, value_type FROM discounts WHERE name = ?`, name).
		Scan(&id, &value, &valueType)
	return
}

type invoiceLine struct {
	itemType  string
	itemID    string
	quantity  int
	amount    float64
	finalAmt  float64
	discounts []string
}

func applyDiscountToLine(ln *invoiceLine, value float64, valueType string) {
	var dv float64
	if valueType == "percentage" {
		dv = ln.amount * value / 100
	} else {
		dv = value
	}
	if dv > ln.amount {
		dv = ln.amount
	}
	ln.finalAmt = ln.amount - dv
}

// Client payments

func seedDemoClientPayments(db *sql.DB, c *demoCtx) error {
	// For about 70% of client invoices, record a payment (full or partial).
	rows, err := db.Query(
		`SELECT i.id, i.invoice_number, i.from_balance_id, i.to_balance_id, i.final_amount, i.created_at
		 FROM invoices i
		 JOIN balances tb ON tb.id = i.to_balance_id
		 WHERE tb.entity_type = 'patient'
		 ORDER BY i.created_at`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type invRow struct {
		id, fromBal, toBal, createdAt string
		number                        int
		finalAmount                   float64
	}
	var invoices []invRow
	for rows.Next() {
		var r invRow
		if err := rows.Scan(&r.id, &r.number, &r.fromBal, &r.toBal, &r.finalAmount, &r.createdAt); err != nil {
			continue
		}
		invoices = append(invoices, r)
	}

	methods := []string{"cash", "card", "transfer", "card", "cash"}
	count := 0
	for i, inv := range invoices {
		mod := i % 10
		if mod == 9 {
			// leave 10% unpaid
			continue
		}
		var amount float64
		desc := ""
		if mod == 7 || mod == 8 {
			// partial payment
			amount = inv.finalAmount / 2
			desc = fmt.Sprintf("Partial payment for Invoice #%d", inv.number)
		} else {
			amount = inv.finalAmount
			desc = fmt.Sprintf("Payment for Invoice #%d", inv.number)
		}
		method := methods[i%len(methods)]
		// Payment is patient to self (reverse of invoice charge direction).
		payTime, _ := time.Parse(time.RFC3339, inv.createdAt)
		payTime = payTime.Add(2 * time.Hour)
		if err := recordTransaction(db, inv.toBal, inv.fromBal, amount, c.currencyID,
			"payment", method, desc, "admin", rfc3339(payTime)); err != nil {
			return err
		}
		count++
	}
	log.Printf("Demo: seeded %d client payments", count)
	return nil
}

// Supplier invoices

func seedDemoSupplierInvoices(db *sql.DB, c *demoCtx) error {
	supplierByName := map[string]string{}
	supplierBal := map[string]string{}
	rows, err := db.Query(
		`SELECT s.id, s.name, b.id
		 FROM suppliers s
		 LEFT JOIN balances b ON b.entity_type = 'supplier' AND b.entity_id = s.id AND b.currency_id = ?`, c.currencyID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var sid, name, bid string
		if err := rows.Scan(&sid, &name, &bid); err != nil {
			continue
		}
		supplierByName[name] = sid
		supplierBal[sid] = bid
	}
	rows.Close()

	type supInv struct {
		supplier string
		offset   int
		items    []struct {
			name string
			qty  int
		}
		paidFrac float64
	}

	purchases := []supInv{
		{"MedSupply Co.", -40, []struct {
			name string
			qty  int
		}{
			{"Botox (Allergan) 100U", 10}, {"Dysport 300U", 5},
			{"Lidocaine 2% 20ml", 20}, {"Nitrile Gloves Medium (100-pack)", 10},
		}, 1.0},
		{"DermaPharma", -35, []struct {
			name string
			qty  int
		}{
			{"Juvederm Voluma XC 1ml", 10}, {"Juvederm Ultra XC 1ml", 10},
			{"Restylane 1ml", 8}, {"Hyaluronidase (Hylenex) 150U", 5},
		}, 1.0},
		{"Aesthetic Essentials", -25, []struct {
			name string
			qty  int
		}{
			{"Profhilo 2ml", 10}, {"Skinvive 1ml", 6}, {"Sculptra (PLLA) Vial", 4},
			{"EMLA Cream 5% 30g", 15},
		}, 0.5},
		{"BioTech Materials", -20, []struct {
			name string
			qty  int
		}{
			{"Morpheus8 Tip 24-pin", 8}, {"Morpheus8 Tip 40-pin", 6},
			{"PRP Collection Kit", 10}, {"Exosome Vial 5ml", 3},
		}, 1.0},
		{"MedSupply Co.", -10, []struct {
			name string
			qty  int
		}{
			{"Amoxicillin 500mg (30 caps)", 10}, {"Dexamethasone 4mg/ml (10 vials)", 5},
			{"Sterile Gauze Pads 4x4 (100-pack)", 10}, {"Alcohol Prep Pads (200-pack)", 10},
		}, 0.0},
		{"DermaPharma", -5, []struct {
			name string
			qty  int
		}{
			{"Radiesse 1.5ml", 6}, {"Belotero Balance 1ml", 6},
		}, 0.0},
	}

	created := 0
	paymentsCreated := 0
	for _, p := range purchases {
		sid, ok := supplierByName[p.supplier]
		if !ok {
			continue
		}
		sbal := supplierBal[sid]
		if sbal == "" {
			continue
		}
		purchaseDate := time.Now().AddDate(0, 0, p.offset)
		ts := rfc3339(purchaseDate)

		num, err := nextInvoiceNumber(db)
		if err != nil {
			return err
		}
		invID := newID()

		// Resolve items + totals
		type resolvedItem struct {
			id    string
			qty   int
			price float64
		}
		var items []resolvedItem
		var total float64
		for _, it := range p.items {
			pid, ok := c.prodByName[it.name]
			if !ok {
				continue
			}
			var price float64
			if err := db.QueryRow(`SELECT unit_price FROM products WHERE id = ?`, pid).Scan(&price); err != nil {
				continue
			}
			line := price * float64(it.qty)
			items = append(items, resolvedItem{pid, it.qty, line})
			total += line
		}

		// Supplier invoice: from = supplier, to = self.
		if _, err := db.Exec(
			`INSERT INTO invoices (id, invoice_number, from_balance_id, to_balance_id, amount, final_amount, currency_id, notes, created_by, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			invID, num, sbal, c.selfBalanceID, total, total, c.currencyID,
			fmt.Sprintf("Restock from %s", p.supplier), "admin", ts, ts,
		); err != nil {
			return err
		}

		for _, it := range items {
			if _, err := db.Exec(
				`INSERT INTO invoice_items (id, invoice_id, item_type, item_id, quantity, amount, final_amount, notes, created_at)
				 VALUES (?,?,?,?,?,?,?,?,?)`,
				newID(), invID, "product", it.id, it.qty, it.price, it.price, "", ts,
			); err != nil {
				return err
			}
			// Stock UP (to=self).
			if _, err := db.Exec(
				`UPDATE products SET quantity = quantity + ? WHERE id = ?`, it.qty, it.id,
			); err != nil {
				return err
			}
		}

		// Balance charge transaction (supplier to self).
		if err := recordTransaction(db, sbal, c.selfBalanceID, total, c.currencyID,
			"charge", "other", fmt.Sprintf("Invoice #%d", num), "admin", ts); err != nil {
			return err
		}
		created++

		// Partial/full payments
		if p.paidFrac > 0 {
			paid := total * p.paidFrac
			payTime := purchaseDate.Add(48 * time.Hour)
			method := "transfer"
			if err := recordTransaction(db, c.selfBalanceID, sbal, paid, c.currencyID,
				"payment", method, fmt.Sprintf("Payment for Invoice #%d", num), "admin", rfc3339(payTime)); err != nil {
				return err
			}
			paymentsCreated++
		}
	}
	log.Printf("Demo: seeded %d supplier invoices (%d with payments)", created, paymentsCreated)
	return nil
}

func seedDemoSupplierPayments(db *sql.DB, c *demoCtx) error {
	// Already handled inline in seedDemoSupplierInvoices; this hook is a
	// placeholder to keep the step list symmetric and readable.
	return nil
}

// Employee payments (salary disbursements)

func seedDemoEmployeePayments(db *sql.DB, c *demoCtx) error {
	// Pay each employee their active salary for the last 2 months + partial month.
	type empPayRow struct {
		empID  string
		salary float64
		name   string
		bal    string
	}
	var rows []empPayRow
	q, err := db.Query(
		`SELECT e.id, e.first_name || ' ' || e.last_name, s.amount, b.id
		 FROM employees e
		 JOIN employee_salaries s ON s.employee_id = e.id AND s.is_active = 1
		 LEFT JOIN balances b ON b.entity_type = 'employee' AND b.entity_id = e.id AND b.currency_id = ?`, c.currencyID)
	if err != nil {
		return err
	}
	for q.Next() {
		var r empPayRow
		if err := q.Scan(&r.empID, &r.name, &r.salary, &r.bal); err == nil && r.bal != "" {
			rows = append(rows, r)
		}
	}
	q.Close()

	count := 0
	for offset := 60; offset >= 30; offset -= 30 {
		payDate := time.Now().AddDate(0, 0, -offset)
		payDate = time.Date(payDate.Year(), payDate.Month(), 1, 9, 0, 0, 0, payDate.Location())
		ts := rfc3339(payDate)
		month := payDate.Format("January 2006")
		for _, r := range rows {
			if err := recordTransaction(db, c.selfBalanceID, r.bal, r.salary, c.currencyID,
				"payment", "transfer", "Salary — "+month, "admin", ts); err != nil {
				return err
			}
			count++
		}
	}
	log.Printf("Demo: seeded %d employee salary payments", count)
	return nil
}

// Bookings (external reservation requests)

func seedDemoBookings(db *sql.DB, c *demoCtx) error {
	type bk struct {
		name, phone, email string
		newClient          bool
		referral           string
		category, service  string
		dateOff            int
		timeStr            string
		durMin             int
		status             string
		notes              string
	}

	bookings := []bk{
		{"Rana Aoun", "+1 555 0180", "rana.aoun@example.com", true, "Instagram",
			"Botox", "Botox Full", 2, "10:00", 45, "pending", "Prefers morning"},
		{"Ziad Abi Saab", "+1 555 0181", "ziad.as@example.com", true, "Google",
			"Face", "Lips", 3, "14:00", 60, "pending", "First-time filler"},
		{"Carla Melki", "+1 555 0182", "carla.m@example.com", false, "Returning",
			"Face", "Profhilo Face", 4, "11:00", 45, "confirmed", "Regular client"},
		{"Anthony Kassab", "+1 555 0183", "anthony.k@example.com", true, "Friend referral",
			"Laser Hair Removal", "Full Body Package 1", 5, "15:00", 90, "pending", ""},
		{"Jessica Khalifeh", "+1 555 0184", "jessica.k@example.com", true, "TikTok",
			"Face", "Cheeks", 6, "13:00", 75, "pending", ""},
		{"Tarek Fares", "+1 555 0185", "tarek.f@example.com", true, "Google",
			"Body", "Body Contouring", 7, "10:00", 60, "pending", "Consult for liposuction"},
		{"Mira Bou Saad", "+1 555 0186", "mira.bs@example.com", true, "Instagram",
			"Quanta Machine", "Tattoo Removal — Face/Body", 8, "12:00", 30, "cancelled", "Cancelled — rescheduling"},
		{"Julian Tannous", "+1 555 0187", "julian.t@example.com", false, "Returning",
			"Botox", "Botox Migraine", 9, "09:30", 45, "confirmed", "Recurring migraine protocol"},
		{"Lara Daccache", "+1 555 0188", "lara.d@example.com", true, "Instagram",
			"Face", "Face & Neck + Plasma", 10, "11:30", 120, "pending", ""},
		{"Rony Bou Nader", "+1 555 0189", "rony.bn@example.com", true, "Walk-in",
			"Hair", "PRP Hair Restoration", 11, "14:30", 75, "pending", "Asked about PRP"},
	}

	now := rfc3339(time.Now())
	for _, b := range bookings {
		isNew := 0
		if b.newClient {
			isNew = 1
		}
		date := dateOnly(time.Now().AddDate(0, 0, b.dateOff))
		if _, err := db.Exec(
			`INSERT INTO bookings (id, client_name, client_phone, client_email, is_new_client, referral_source,
				service_category, service_name, preferred_date, preferred_time, duration_minutes, status,
				appointment_id, patient_id, room_id, notes, created_at, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,'','','',?,?,?)`,
			newID(), b.name, b.phone, b.email, isNew, b.referral,
			b.category, b.service, date, b.timeStr, b.durMin, b.status, b.notes, now, now,
		); err != nil {
			return err
		}
	}
	log.Printf("Demo: seeded %d bookings", len(bookings))
	return nil
}

// Notifications

func seedDemoNotifications(db *sql.DB, c *demoCtx) error {
	type note struct {
		title, desc, action string
		isRead              int
		agoHours            int
	}
	notes := []note{
		{"New booking received", "Rana Aoun requested Botox Full in 2 days", "/bookings", 0, 1},
		{"Low stock: Juvederm Voluma XC 1ml", "Only 5 units remaining (min threshold 5)", "/inventory", 0, 3},
		{"Low stock: Hyaluronidase (Hylenex) 150U", "Only 3 units remaining", "/inventory", 0, 6},
		{"New booking received", "Ziad Abi Saab requested Lips filler in 3 days", "/bookings", 0, 8},
		{"Payment received", "Rami Chahine paid $250 for Invoice #12", "/transactions", 1, 24},
		{"Supplier invoice overdue", "DermaPharma invoice from 35 days ago still unpaid", "/transactions", 0, 30},
		{"Voucher redeemed", "BOOST-A1B2 redeemed for a skin booster session", "/services/discounts", 1, 48},
		{"Appointment cancelled", "Sarah Moukarzel cancelled tomorrow's laser slot", "/appointments", 1, 72},
		{"Booking confirmed", "Carla Melki's Profhilo booking confirmed for this week", "/bookings", 1, 96},
		{"New prescription created", "Dr. Julian prescribed medications for Marc Tabet", "/patients", 1, 120},
	}
	for _, n := range notes {
		at := time.Now().Add(-time.Duration(n.agoHours) * time.Hour)
		if _, err := db.Exec(
			`INSERT INTO notifications (id, user_id, title, description, action, is_read, created_at)
			 VALUES (?,?,?,?,?,?,?)`,
			newID(), c.adminUserID, n.title, n.desc, n.action, n.isRead, rfc3339(at),
		); err != nil {
			return err
		}
	}
	log.Printf("Demo: seeded %d notifications", len(notes))
	return nil
}
