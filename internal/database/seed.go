package database

import (
	"clinic-api/internal/utils"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// SeedIfEmpty seeds the database with initial data if the rooms table is empty.
// Seed order: counters, roles, rooms, allergies, patients, patient_allergies,
// appointments, inventory, stock_adjustments, team_members, users, procedures, balances
func SeedIfEmpty(db *sql.DB) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count)
	if err != nil {
		return fmt.Errorf("check seed: %w", err)
	}
	if count > 0 {
		log.Println("Database already seeded, skipping")
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	nowStr := now.Format(time.RFC3339)

	// Counters
	if _, err := tx.Exec(`INSERT INTO counters (name, value) VALUES ('patient', 5), ('team_member', 5), ('invoice', 0)`); err != nil {
		return fmt.Errorf("seed counters: %w", err)
	}

	// Roles
	type roleSeed struct{ name, label, scopes string }
	roles := []roleSeed{
		{"super-admin", "Super Admin — Developer", `["dashboard:read","patients:read","patients:write","patients:delete","appointments:read","appointments:write","appointments:delete","inventory:read","inventory:write","transactions:read","transactions:write","services:read","services:write","team:read","team:write","team:delete","team:view-salaries","reports:read","rooms:read","roles:read","roles:write","bookings:read","bookings:write"]`},
		{"admin", "Admin — Doctor / Support", `["dashboard:read","patients:read","patients:write","patients:delete","appointments:read","appointments:write","appointments:delete","inventory:read","inventory:write","transactions:read","transactions:write","services:read","services:write","team:read","team:write","team:delete","team:view-salaries","reports:read","rooms:read","roles:read","bookings:read","bookings:write"]`},
		{"user", "Clinic Staff", `["dashboard:read","patients:read","appointments:read","appointments:write","inventory:read","transactions:read","services:read","team:read","reports:read","rooms:read","bookings:read"]`},
	}
	for _, r := range roles {
		if _, err := tx.Exec(`INSERT INTO roles (name, label, scopes) VALUES (?, ?, ?)`, r.name, r.label, r.scopes); err != nil {
			return fmt.Errorf("seed role %s: %w", r.name, err)
		}
	}

	// Rooms
	rooms := []struct{ id, name, typ string }{
		{"room-1", "Room 1", "Consultation"},
		{"room-2", "Room 2", "Procedure"},
		{"room-3", "Room 3", "General"},
		{"room-4", "Room 4", "Consultation"},
		{"room-5", "Room 5", "Procedure"},
		{"room-6", "Room 6", "General"},
		{"room-7", "Room 7", "Consultation"},
		{"room-hospital", "Hospital", "Hospital"},
	}
	for _, r := range rooms {
		if _, err := tx.Exec(`INSERT INTO rooms (id, name, type, is_available, created_at) VALUES (?, ?, ?, 1, ?)`, r.id, r.name, r.typ, nowStr); err != nil {
			return fmt.Errorf("seed room %s: %w", r.id, err)
		}
	}

	// Allergies
	allergies := []struct{ id, name, desc string }{
		{"allergy-001", "Penicillin", "Allergy to penicillin-class antibiotics"},
		{"allergy-002", "Latex", "Allergy to latex gloves and materials"},
		{"allergy-003", "Sulfa", "Allergy to sulfonamide medications"},
		{"allergy-004", "Aspirin", "Allergy to aspirin/NSAIDs"},
		{"allergy-005", "Lidocaine", "Allergy to local anesthetics"},
		{"allergy-006", "Hyaluronic Acid", "Allergy to HA-based fillers"},
		{"allergy-007", "Botulinum Toxin", "Allergy to botox/botulinum products"},
	}
	for _, a := range allergies {
		if _, err := tx.Exec(`INSERT INTO allergies (id, name, description, created_at) VALUES (?,?,?,?)`,
			a.id, a.name, a.desc, nowStr); err != nil {
			return fmt.Errorf("seed allergy %s: %w", a.id, err)
		}
	}

	// Patients
	type patientSeed struct {
		id, firstName, middleName, lastName, gender, dob, contact, email string
		ecName, ecPhone                                                  string
		weight, height                                                   float64
		bp, bloodType, physicalActivity                                  string
		isSmoker                                                         int
		packsPerDay                                                      float64
		onHerbal, onMed                                                  int
		medDetails                                                       string
		onBloodThinners, onHRT                                           int
		hrtDetails                                                       string
		patientNumber, daysAgo                                           int
	}
	patients := []patientSeed{
		{"PAT-001", "Sarah", "", "Johnson", "Female", "1985-03-15", "+1 555-0101", "sarah.j@example.com",
			"Tom Johnson", "+1 555-0198", 65, 165, "120/80", "A+", "Moderate",
			0, 0, 0, 0, "", 0, 0, "", 1, 90},
		{"PAT-002", "Michael", "J.", "Chen", "Male", "1978-07-22", "+1 555-0102", "m.chen@example.com",
			"Linda Chen", "+1 555-0199", 82, 178, "130/85", "O+", "Light",
			1, 1, 0, 1, "Lisinopril 10mg daily", 0, 0, "", 2, 60},
		{"PAT-003", "Emily", "A.", "Rodriguez", "Female", "1992-03-18", "+1 555-0103", "emily.r@example.com",
			"Carlos Rodriguez", "+1 555-0116", 58, 160, "118/75", "B-", "Active",
			0, 0, 1, 0, "", 0, 1, "Estradiol patch", 3, 45},
		{"PAT-004", "James", "", "Wilson", "Male", "1960-01-18", "+1 555-0104", "j.wilson@example.com",
			"Mary Wilson", "+1 555-0117", 95, 182, "145/92", "AB+", "Sedentary",
			0, 0, 0, 1, "Metoprolol 50mg, Atorvastatin 20mg", 1, 0, "", 4, 30},
		{"PAT-005", "Aisha", "", "Patel", "Female", "1995-09-30", "+1 555-0105", "aisha.p@example.com",
			"Raj Patel", "+1 555-0119", 52, 155, "110/70", "O-", "Very Active",
			0, 0, 0, 0, "", 0, 0, "", 5, 15},
	}
	for _, p := range patients {
		createdAt := today.AddDate(0, 0, -p.daysAgo).Format(time.RFC3339)
		if _, err := tx.Exec(`INSERT INTO patients (
			id, patient_number, first_name, middle_name, last_name, gender, date_of_birth,
			contact, email, emergency_contact_name, emergency_contact_phone,
			weight, height, bp, blood_type, physical_activity,
			is_smoker, packs_per_day, on_herbal_supplements, on_medication, medication_details,
			on_blood_thinners, on_hrt, hrt_details, notes, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.id, p.patientNumber, p.firstName, p.middleName, p.lastName, p.gender, p.dob,
			p.contact, p.email, p.ecName, p.ecPhone,
			p.weight, p.height, p.bp, p.bloodType, p.physicalActivity,
			p.isSmoker, p.packsPerDay, p.onHerbal, p.onMed, p.medDetails,
			p.onBloodThinners, p.onHRT, p.hrtDetails, "", createdAt, createdAt,
		); err != nil {
			return fmt.Errorf("seed patient %s: %w", p.id, err)
		}
	}

	// Patient Allergies
	patientAllergies := []struct{ id, patientID, allergyID string }{
		{"pa-001", "PAT-001", "allergy-001"}, // Sarah - Penicillin
		{"pa-002", "PAT-003", "allergy-002"}, // Emily - Latex
		{"pa-003", "PAT-003", "allergy-003"}, // Emily - Sulfa
		{"pa-004", "PAT-004", "allergy-004"}, // James - Aspirin
	}
	for _, pa := range patientAllergies {
		if _, err := tx.Exec(`INSERT INTO patient_allergies (id, patient_id, allergy_id, notes, created_at) VALUES (?,?,?,?,?)`,
			pa.id, pa.patientID, pa.allergyID, "", nowStr); err != nil {
			return fmt.Errorf("seed patient allergy %s: %w", pa.id, err)
		}
	}

	// Appointments
	type aptSeed struct {
		id, patientId, roomId                                        string
		dayOffset, startHour, startMin, endHour, endMin              int
		treatmentType, status, notes                                  string
	}
	apts := []aptSeed{
		{"apt-001", "PAT-001", "room-1", 0, 9, 0, 9, 30, "Consultation", "Completed", "Routine checkup"},
		{"apt-002", "PAT-002", "room-2", 0, 10, 0, 11, 0, "Procedure", "In-Progress", "Minor procedure"},
		{"apt-003", "PAT-003", "room-1", 0, 14, 0, 14, 30, "Follow-up", "Scheduled", "Post-procedure follow-up"},
		{"apt-004", "PAT-004", "room-3", 0, 11, 0, 11, 30, "Consultation", "Scheduled", "BP monitoring"},
		{"apt-005", "PAT-005", "room-1", 1, 9, 0, 9, 30, "Consultation", "Scheduled", "New patient intake"},
		{"apt-006", "PAT-001", "room-2", 1, 13, 0, 14, 0, "Procedure", "Scheduled", "Scheduled procedure"},
		{"apt-007", "PAT-002", "room-3", 2, 10, 0, 10, 30, "Follow-up", "Scheduled", "Follow-up appointment"},
	}
	for _, a := range apts {
		day := today.AddDate(0, 0, a.dayOffset)
		startTime := time.Date(day.Year(), day.Month(), day.Day(), a.startHour, a.startMin, 0, 0, time.Local).Format(time.RFC3339)
		endTime := time.Date(day.Year(), day.Month(), day.Day(), a.endHour, a.endMin, 0, 0, time.Local).Format(time.RFC3339)
		if _, err := tx.Exec(`INSERT INTO appointments (id, patient_id, room_id, employee_id, patient_procedure_session_id,
			start_time, end_time, treatment_type, status, approval_status, notes, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			a.id, a.patientId, a.roomId, "", "", startTime, endTime, a.treatmentType, a.status,
			"not_required", a.notes, nowStr, nowStr); err != nil {
			return fmt.Errorf("seed appointment %s: %w", a.id, err)
		}
	}

	// Inventory
	type invSeed struct {
		sku, name string
		qty, min  int
		price     float64
		category  string
		daysAgo   int
	}
	inventory := []invSeed{
		{"MED-001", "Paracetamol 500mg", 240, 50, 0.12, "Medication", 5},
		{"MED-002", "Amoxicillin 250mg", 80, 30, 0.45, "Medication", 12},
		{"CON-001", "Surgical Gloves (Box)", 15, 10, 8.5, "Consumable", 3},
		{"CON-002", "Bandage Roll 5cm", 45, 20, 2.25, "Consumable", 8},
		{"CON-003", "Alcohol Swabs (100pk)", 8, 10, 4.5, "Consumable", 15},
		{"EQP-001", "Digital Thermometer", 5, 2, 25.0, "Equipment", 30},
		{"MED-003", "Ibuprofen 400mg", 22, 40, 0.18, "Medication", 20},
		{"CON-004", "Syringes 5ml (Box)", 30, 15, 12.0, "Consumable", 7},
	}
	for _, i := range inventory {
		restocked := today.AddDate(0, 0, -i.daysAgo).Format(time.RFC3339)
		if _, err := tx.Exec(`INSERT INTO inventory_items (sku, name, category_id, quantity, min_threshold, unit_price, category, last_restocked, created_at)
			VALUES (?,?,?,?,?,?,?,?,?)`, i.sku, i.name, "", i.qty, i.min, i.price, i.category, restocked, nowStr); err != nil {
			return fmt.Errorf("seed inventory %s: %w", i.sku, err)
		}
	}

	// Stock Adjustments
	adjs := []struct {
		id, sku, typ string
		qty          int
		reason       string
		daysAgo      int
	}{
		{"adj-001", "MED-001", "Purchase", 100, "Monthly restock", 5},
		{"adj-002", "CON-003", "Damage", -5, "Water damage", 10},
		{"adj-003", "MED-003", "Adjustment", -8, "Inventory correction", 15},
	}
	for _, a := range adjs {
		ts := today.AddDate(0, 0, -a.daysAgo).Format(time.RFC3339)
		if _, err := tx.Exec(`INSERT INTO stock_adjustments (id, sku, type, quantity_change, reason, timestamp) VALUES (?,?,?,?,?,?)`,
			a.id, a.sku, a.typ, a.qty, a.reason, ts); err != nil {
			return fmt.Errorf("seed adjustment %s: %w", a.id, err)
		}
	}

	// Team Members
	type tmSeed struct {
		id, firstName, lastName, role, contact, email, dob string
		empType                                            string
		salary                                             float64
		schedule, offDays, hireDate, status                string
	}
	members := []tmSeed{
		{"TM-001", "Julian", "Vance", "Doctor", "+1 555-0133", "julian.vance@example.org", "1980-06-10",
			"Full-time", 15000, `["Monday","Tuesday","Wednesday","Thursday","Friday"]`, `["2026-03-20"]`, "2020-01-15", "Active"},
		{"TM-002", "Maria", "Santos", "Nurse", "+1 555-0135", "maria.s@example.org", "1990-03-17",
			"Full-time", 5500, `["Monday","Tuesday","Wednesday","Thursday","Friday"]`, `[]`, "2021-06-01", "Active"},
		{"TM-003", "David", "Kim", "Lab Technician", "+1 555-0137", "david.k@example.org", "1995-08-22",
			"Part-time", 2800, `["Monday","Wednesday","Friday"]`, `["2026-03-18"]`, "2023-03-10", "Active"},
		{"TM-004", "Linda", "Nguyen", "Receptionist", "+1 555-0139", "linda.n@example.org", "1988-12-03",
			"Full-time", 3200, `["Monday","Tuesday","Wednesday","Thursday","Friday","Saturday"]`, `[]`, "2022-09-15", "Active"},
		{"TM-005", "Ahmed", "Hassan", "Nurse", "+1 555-0141", "ahmed.h@example.org", "1993-04-14",
			"Part-time", 2600, `["Tuesday","Thursday","Saturday"]`, `["2026-04-01","2026-04-02"]`, "2024-01-20", "Inactive"},
	}
	for _, m := range members {
		if _, err := tx.Exec(`INSERT INTO team_members (id, user_id, first_name, last_name, role, contact, email, date_of_birth,
			employment_type, salary, schedule, off_days, hire_date, status, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			m.id, "", m.firstName, m.lastName, m.role, m.contact, m.email, m.dob,
			m.empType, m.salary, m.schedule, m.offDays, m.hireDate, m.status, nowStr, nowStr); err != nil {
			return fmt.Errorf("seed team member %s: %w", m.id, err)
		}
	}

	// Users
	type userSeed struct{ id, username, password, displayName, role string }
	users := []userSeed{
		{"usr-001", "admin", "admin123", "Dr. Julian Vance", "super-admin"},
		{"usr-002", "doctor", "doctor123", "Dr. Julian Vance", "admin"},
		{"usr-003", "staff", "staff123", "Clinic Staff", "user"},
	}
	for _, u := range users {
		hash, err := utils.HashPassword(u.password)
		if err != nil {
			return fmt.Errorf("hash password for %s: %w", u.username, err)
		}
		if _, err := tx.Exec(`INSERT INTO users (id, username, password_hash, display_name, role) VALUES (?,?,?,?,?)`,
			u.id, u.username, hash, u.displayName, u.role); err != nil {
			return fmt.Errorf("seed user %s: %w", u.username, err)
		}
	}

	// Procedures
	type procSeed struct {
		name, procType, category, subcategory string
		durationMinutes                       int
		commissionRate                        float64
		remarks                               string
		// Single session price (stored as session 1)
		price float64
	}
	procedures := []procSeed{
		// Botox - Women
		{"Botox Full", "Clinic Procedure", "Botox", "Women", 30, 0, "", 220},
		{"Botox Full (Dysport)", "Clinic Procedure", "Botox", "Women", 30, 0, "", 250},
		{"Botox Full + Bunny Lines", "Clinic Procedure", "Botox", "Women", 30, 0, "", 250},
		{"Botox Around Eyes", "Clinic Procedure", "Botox", "Women", 20, 0, "", 120},
		{"Botox Frown Lines", "Clinic Procedure", "Botox", "Women", 20, 0, "", 120},
		{"Botox Gummy Smile", "Clinic Procedure", "Botox", "Women", 15, 0, "", 120},
		{"Botox Clenching Teeth", "Clinic Procedure", "Botox", "Women", 20, 0, "", 350},
		{"Botox Sweating", "Clinic Procedure", "Botox", "Women", 30, 0, "", 350},
		{"Botox Migraine", "Clinic Procedure", "Botox", "Women", 30, 0, "", 400},
		{"Botox Neck", "Clinic Procedure", "Botox", "Women", 30, 0, "", 220},
		{"Botox Calves", "Clinic Procedure", "Botox", "Women", 30, 0, "", 350},
		{"Botox Jaw", "Clinic Procedure", "Botox", "Women", 20, 0, "", 150},
		{"Traptox", "Clinic Procedure", "Botox", "Women", 30, 0, "", 350},
		{"Lip Flip", "Clinic Procedure", "Botox", "Women", 15, 0, "", 120},
		// Fillers
		{"Lips Filler", "Clinic Procedure", "Fillers", "Face", 30, 0, "", 250},
		{"Nasolabial Folds", "Clinic Procedure", "Fillers", "Face", 30, 0, "", 350},
		{"Cheeks Filler", "Clinic Procedure", "Fillers", "Face", 45, 0, "", 350},
		{"Jawline Filler", "Clinic Procedure", "Fillers", "Face", 45, 0, "", 350},
		{"Chin Filler", "Clinic Procedure", "Fillers", "Face", 30, 0, "", 300},
		{"Under Eyes Filler", "Clinic Procedure", "Fillers", "Face", 30, 0, "", 350},
		{"Nose (Non-Surgical)", "Clinic Procedure", "Fillers", "Face", 30, 0, "", 350},
		// Skin Boosters
		{"Profhilo Face", "Clinic Procedure", "Skin Boosters", "Face", 30, 0, "", 250},
		{"Profhilo Neck", "Clinic Procedure", "Skin Boosters", "Face", 30, 0, "", 250},
		{"Skinvive", "Clinic Procedure", "Skin Boosters", "Face", 30, 0, "", 250},
		{"Sculptra", "Clinic Procedure", "Skin Boosters", "Face", 45, 0, "", 350},
		// Morpheus8
		{"Morpheus8 Face + Plasma", "Clinic Procedure", "Morpheus8", "Face", 60, 0, "", 350},
		{"Morpheus8 Neck + Plasma", "Clinic Procedure", "Morpheus8", "Face", 45, 0, "", 250},
		{"Morpheus8 Face & Neck + Plasma", "Clinic Procedure", "Morpheus8", "Face", 90, 0, "", 500},
		// Laser Hair Removal
		{"Laser Full Face", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 30, 0, "", 60},
		{"Laser Underarms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 15, 0, "", 25},
		{"Laser Full Legs", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, 0, "", 80},
		{"Laser Brazilian", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 30, 0, "", 60},
		{"Laser Full Body Package", "Clinic Procedure", "Laser Hair Removal", "Packages", 120, 0, "", 250},
		// Hospital Surgery
		{"Rhinoplasty", "Hospital Surgery", "Face", "Nose", 180, 0, "General anesthesia", 0},
		{"Blepharoplasty", "Hospital Surgery", "Face", "Eyes", 120, 0, "Local or general anesthesia", 0},
		{"Liposuction", "Hospital Surgery", "Body", "Contouring", 180, 0, "General anesthesia", 0},
		{"Breast Augmentation", "Hospital Surgery", "Body", "Breast", 180, 0, "General anesthesia", 0},
		// Minor Surgery
		{"Mole Removal", "Minor Surgery", "Skin", "Lesions", 30, 0, "Local anesthesia", 150},
		{"Cyst Removal", "Minor Surgery", "Skin", "Lesions", 45, 0, "Local anesthesia", 200},
		// Others
		{"Consultation with Dr. Joe", "Clinic Procedure", "Others", "General", 30, 0, "", 50},
		{"PRP (Platelet-Rich Plasma)", "Clinic Procedure", "Others", "General", 45, 0, "", 200},
		{"Filler Dissolver", "Clinic Procedure", "Others", "General", 30, 0, "", 150},
	}
	for i, p := range procedures {
		id := fmt.Sprintf("proc-%03d", i+1)
		if _, err := tx.Exec(`INSERT INTO procedures (id, name, procedure_type, category, subcategory,
			duration_minutes, commission_rate, commission_type, is_active, remarks, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, p.name, p.procType, p.category, p.subcategory,
			p.durationMinutes, p.commissionRate, "percentage", 1, p.remarks, nowStr, nowStr,
		); err != nil {
			return fmt.Errorf("seed procedure %s: %w", p.name, err)
		}
		// Create a single session with the price
		if p.price > 0 {
			sessID := fmt.Sprintf("psess-%03d-1", i+1)
			if _, err := tx.Exec(`INSERT INTO procedure_sessions (id, procedure_id, session_number, name, description, duration_minutes, price, currency, created_at)
				VALUES (?,?,?,?,?,?,?,?,?)`,
				sessID, id, 1, "Session 1", "", p.durationMinutes, p.price, "USD", nowStr,
			); err != nil {
				return fmt.Errorf("seed procedure session for %s: %w", p.name, err)
			}
		}
	}

	// Balances: create clinic self-balance
	if _, err := tx.Exec(`INSERT INTO balances (id, entity_type, entity_id, entity_name, currency, amount, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, "bal-self-usd", "self", "clinic", "ZEAL Clinic", "USD", 0, nowStr, nowStr); err != nil {
		return fmt.Errorf("seed self balance: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO balances (id, entity_type, entity_id, entity_name, currency, amount, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, "bal-self-lbp", "self", "clinic", "ZEAL Clinic", "LBP", 0, nowStr, nowStr); err != nil {
		return fmt.Errorf("seed self balance LBP: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed: %w", err)
	}

	log.Println("Database seeded successfully")
	return nil
}

// SeedUsersIfEmpty creates default user accounts if the users table is empty.
func SeedUsersIfEmpty(db *sql.DB) error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return fmt.Errorf("check users: %w", err)
	}
	if count > 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	type userSeed struct{ id, username, password, displayName, role string }
	users := []userSeed{
		{"usr-001", "admin", "admin123", "Dr. Julian Vance", "super-admin"},
		{"usr-002", "doctor", "doctor123", "Dr. Julian Vance", "admin"},
		{"usr-003", "staff", "staff123", "Clinic Staff", "user"},
	}
	for _, u := range users {
		hash, err := utils.HashPassword(u.password)
		if err != nil {
			return fmt.Errorf("hash password for %s: %w", u.username, err)
		}
		if _, err := tx.Exec(`INSERT INTO users (id, username, password_hash, display_name, role) VALUES (?,?,?,?,?)`,
			u.id, u.username, hash, u.displayName, u.role); err != nil {
			return fmt.Errorf("seed user %s: %w", u.username, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user seed: %w", err)
	}
	log.Println("Default user accounts created (admin/admin123, doctor/doctor123, staff/staff123)")
	return nil
}
