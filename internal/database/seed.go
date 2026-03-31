package database

import (
	"database/sql"
	"encoding/json"
	"log"

	"clinic-api/internal/utils"

	"github.com/google/uuid"
)

func seedRoles(db *sql.DB) error {
	allScopes := "appointments:read,appointments:write,appointments:delete," +
		"patients:read,patients:write,patients:delete," +
		"team:read,team:write,team:delete," +
		"transactions:read,transactions:write,transactions:delete," +
		"inventory:read,inventory:write,inventory:delete," +
		"services:read,services:write," +
		"rooms:read,rooms:write,rooms:delete," +
		"bookings:read,bookings:write," +
		"roles:read,roles:write," +
		"reports:read"

	roles := []struct {
		name, label, scopes string
	}{
		{"super-admin", "Super Admin", allScopes},
		{"admin", "Admin", allScopes},
		{"user", "User", "appointments:read,patients:read,team:read,rooms:read,bookings:read,services:read,inventory:read,reports:read"},
	}

	for _, r := range roles {
		_, err := db.Exec(
			`INSERT INTO roles (name, label, scopes) VALUES (?, ?, ?) ON CONFLICT(name) DO NOTHING`,
			r.name, r.label, r.scopes,
		)
		if err != nil {
			return err
		}
	}

	log.Println("Seeded default roles")
	return nil
}

func seedProcedures(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM procedures").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type sessionSeed struct {
		Label string  `json:"label"`
		Price float64 `json:"price"`
	}

	type svcSeed struct {
		name, procType, category, subcategory string
		price                                 float64
		priceNote, remarks                    string
		sessionPricing, includes              string
	}

	services := []svcSeed{
		// Clinic Procedure: Botox (Women)
		{"Botox Full", "Clinic Procedure", "Botox", "Women", 220, "", "", "[]", "[]"},
		{"Botox Full (Dysport)", "Clinic Procedure", "Botox", "Women", 250, "", "", "[]", "[]"},
		{"Botox Full + Bunny Lines", "Clinic Procedure", "Botox", "Women", 250, "", "", "[]", "[]"},
		{"Botox Marionette Lines (DAO)", "Clinic Procedure", "Botox", "Women", 30, "", "", "[]", "[]"},
		{"Botox Around Eyes", "Clinic Procedure", "Botox", "Women", 120, "", "", "[]", "[]"},
		{"Botox Frown Lines", "Clinic Procedure", "Botox", "Women", 120, "", "", "[]", "[]"},
		{"Botox Gummy Smile", "Clinic Procedure", "Botox", "Women", 120, "", "", "[]", "[]"},
		{"Botox Clenching Teeth", "Clinic Procedure", "Botox", "Women", 350, "", "", "[]", "[]"},
		{"Botox Sweating", "Clinic Procedure", "Botox", "Women", 350, "", "", "[]", "[]"},
		{"Botox Migraine", "Clinic Procedure", "Botox", "Women", 400, "", "", "[]", "[]"},
		{"Botox Neck", "Clinic Procedure", "Botox", "Women", 220, "", "", "[]", "[]"},
		{"Botox Calves", "Clinic Procedure", "Botox", "Women", 350, "", "", "[]", "[]"},
		{"Botox Jaw", "Clinic Procedure", "Botox", "Women", 150, "", "", "[]", "[]"},
		{"Traptox", "Clinic Procedure", "Botox", "Women", 350, "", "", "[]", "[]"},
		{"Lip Flip", "Clinic Procedure", "Botox", "Women", 120, "", "", "[]", "[]"},
		// Clinic Procedure: Botox (Men)
		{"Botox Full", "Clinic Procedure", "Botox", "Men", 250, "", "", "[]", "[]"},
		{"Botox Full (Dysport)", "Clinic Procedure", "Botox", "Men", 280, "", "", "[]", "[]"},
		{"Botox Full + Bunny Lines", "Clinic Procedure", "Botox", "Men", 280, "", "", "[]", "[]"},
		{"Botox Marionette Lines (DAO)", "Clinic Procedure", "Botox", "Men", 30, "", "", "[]", "[]"},
		{"Botox Around Eyes", "Clinic Procedure", "Botox", "Men", 150, "", "", "[]", "[]"},
		{"Botox Frown Lines", "Clinic Procedure", "Botox", "Men", 150, "", "", "[]", "[]"},
		{"Botox Gummy Smile", "Clinic Procedure", "Botox", "Men", 120, "", "", "[]", "[]"},
		{"Botox Teeth Clenching", "Clinic Procedure", "Botox", "Men", 350, "", "", "[]", "[]"},
		{"Botox Sweating", "Clinic Procedure", "Botox", "Men", 350, "", "", "[]", "[]"},
		{"Botox Migraine", "Clinic Procedure", "Botox", "Men", 400, "", "", "[]", "[]"},
		{"Botox Neck", "Clinic Procedure", "Botox", "Men", 250, "", "", "[]", "[]"},
		{"Botox Calves", "Clinic Procedure", "Botox", "Men", 350, "", "", "[]", "[]"},
		{"Botox Jaw", "Clinic Procedure", "Botox", "Men", 180, "", "", "[]", "[]"},
		{"Traptox", "Clinic Procedure", "Botox", "Men", 350, "", "", "[]", "[]"},
		{"Lip Flip", "Clinic Procedure", "Botox", "Men", 120, "", "", "[]", "[]"},
		// Clinic Procedure: Fillers
		{"Lips", "Clinic Procedure", "Fillers", "Face", 250, "", "", "[]", "[]"},
		{"Nasolabial Folds", "Clinic Procedure", "Fillers", "Face", 350, "", "", "[]", "[]"},
		{"Marionette Lines", "Clinic Procedure", "Fillers", "Face", 350, "", "", "[]", "[]"},
		{"Cheeks", "Clinic Procedure", "Fillers", "Face", 350, "", "", "[]", "[]"},
		{"Jawline", "Clinic Procedure", "Fillers", "Face", 350, "", "", "[]", "[]"},
		{"Chin", "Clinic Procedure", "Fillers", "Face", 300, "", "", "[]", "[]"},
		{"Under Eyes", "Clinic Procedure", "Fillers", "Face", 350, "", "", "[]", "[]"},
		{"Nose (Non-Surgical)", "Clinic Procedure", "Fillers", "Face", 350, "", "", "[]", "[]"},
		{"Temples", "Clinic Procedure", "Fillers", "Face", 350, "", "", "[]", "[]"},
		{"Earlobes", "Clinic Procedure", "Fillers", "Face", 200, "", "", "[]", "[]"},
		{"Hands", "Clinic Procedure", "Fillers", "Body", 350, "", "", "[]", "[]"},
		// Clinic Procedure: Skin Boosters
		{"Profhilo Face", "Clinic Procedure", "Skin Boosters", "Face", 250, "", "", "[]", "[]"},
		{"Profhilo Neck", "Clinic Procedure", "Skin Boosters", "Face", 250, "", "", "[]", "[]"},
		{"Skinvive", "Clinic Procedure", "Skin Boosters", "Face", 250, "", "", "[]", "[]"},
		{"Sculptra", "Clinic Procedure", "Skin Boosters", "Face", 350, "", "", "[]", "[]"},
		{"Exosome", "Clinic Procedure", "Skin Boosters", "Face", 350, "", "", "[]", "[]"},
		{"Collagen Booster", "Clinic Procedure", "Skin Boosters", "Face", 250, "", "", "[]", "[]"},
		{"Jalupro Face", "Clinic Procedure", "Skin Boosters", "Face", 200, "", "", "[]", "[]"},
		{"Jalupro Neck", "Clinic Procedure", "Skin Boosters", "Face", 200, "", "", "[]", "[]"},
		{"Profhilo Body", "Clinic Procedure", "Skin Boosters", "Body", 350, "", "", "[]", "[]"},
		{"Jalupro Eye", "Clinic Procedure", "Skin Boosters", "Eyes", 200, "", "", "[]", "[]"},
		{"Chroma Phill Art Eye", "Clinic Procedure", "Skin Boosters", "Eyes", 200, "", "", "[]", "[]"},
		// Clinic Procedure: Morpheus8
		{"Face + Plasma", "Clinic Procedure", "Morpheus8", "Face", 350, "per session", "", `[{"label":"1 Session","price":350},{"label":"3 Sessions","price":900},{"label":"4 Sessions","price":1100}]`, "[]"},
		{"Neck + Plasma", "Clinic Procedure", "Morpheus8", "Face", 250, "per session", "", `[{"label":"1 Session","price":250},{"label":"3 Sessions","price":650}]`, "[]"},
		{"Face & Neck + Plasma", "Clinic Procedure", "Morpheus8", "Face", 500, "per session", "", `[{"label":"1 Session","price":500},{"label":"3 Sessions","price":1300}]`, "[]"},
		{"Arms", "Clinic Procedure", "Morpheus8", "Body", 350, "per session", "", "[]", "[]"},
		{"Abdominal", "Clinic Procedure", "Morpheus8", "Body", 500, "per session", "", "[]", "[]"},
		{"Love Handles", "Clinic Procedure", "Morpheus8", "Body", 350, "per session", "", "[]", "[]"},
		{"Thighs", "Clinic Procedure", "Morpheus8", "Body", 500, "per session", "", "[]", "[]"},
		{"Buttocks", "Clinic Procedure", "Morpheus8", "Body", 500, "per session", "", "[]", "[]"},
		{"Hands", "Clinic Procedure", "Morpheus8", "Body", 250, "per session", "", "[]", "[]"},
		{"Knees", "Clinic Procedure", "Morpheus8", "Body", 250, "per session", "", "[]", "[]"},
		// Clinic Procedure: Laser Hair Removal
		{"Full Face", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", "[]", "[]"},
		{"Upper Lip", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 15, "", "", "[]", "[]"},
		{"Sideburns", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 20, "", "", "[]", "[]"},
		{"Chin", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 15, "", "", "[]", "[]"},
		{"Full Arms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", "[]", "[]"},
		{"Half Arms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", "[]", "[]"},
		{"Underarms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 25, "", "", "[]", "[]"},
		{"Full Legs", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 80, "", "", "[]", "[]"},
		{"Half Legs", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", "[]", "[]"},
		{"Bikini", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", "[]", "[]"},
		{"Brazilian", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", "[]", "[]"},
		{"Chest", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", "[]", "[]"},
		{"Abdomen", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", "[]", "[]"},
		{"Full Back", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", "[]", "[]"},
		{"Half Back", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", "[]", "[]"},
		{"Neck", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 25, "", "", "[]", "[]"},
		{"Buttocks", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", "[]", "[]"},
		{"Hands / Feet", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 20, "", "", "[]", "[]"},
		{"Full Body Package 1", "Clinic Procedure", "Laser Hair Removal", "Packages", 200, "", "", "[]", `["Full Face","Underarms","Full Arms","Brazilian"]`},
		{"Full Body Package 2", "Clinic Procedure", "Laser Hair Removal", "Packages", 250, "", "", "[]", `["Full Face","Underarms","Full Arms","Brazilian","Full Legs"]`},
		{"Full Body Package 3", "Clinic Procedure", "Laser Hair Removal", "Packages", 300, "", "", "[]", `["Full Face","Underarms","Full Arms","Brazilian","Full Legs","Full Back"]`},
		// Clinic Procedure: Quanta Machine
		{"Tattoo Removal — Eyebrows", "Clinic Procedure", "Quanta Machine", "Tattoo Removal", 100, "", "", "[]", "[]"},
		{"Tattoo Removal — Face/Body", "Clinic Procedure", "Quanta Machine", "Tattoo Removal", 150, "", "", "[]", "[]"},
		{"Varicose Vein — Per Vein", "Clinic Procedure", "Quanta Machine", "Varicose", 50, "per vein", "", "[]", "[]"},
		{"Varicose — Full Face", "Clinic Procedure", "Quanta Machine", "Varicose", 150, "", "", "[]", "[]"},
		{"Varicose — Full Body", "Clinic Procedure", "Quanta Machine", "Varicose", 300, "", "", "[]", "[]"},
		{"Melasma Q-Switched", "Clinic Procedure", "Quanta Machine", "Melasma", 100, "", "", "[]", "[]"},
		{"Melasma Full Face", "Clinic Procedure", "Quanta Machine", "Melasma", 200, "", "", "[]", "[]"},
		{"Carbon Peel", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", "[]", "[]"},
		{"Hair Bleaching", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", "[]", "[]"},
		{"Rosacea Treatment", "Clinic Procedure", "Quanta Machine", "Other", 150, "", "", "[]", "[]"},
		{"Scar Treatment", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", "[]", "[]"},
		{"Cherry Angiomas", "Clinic Procedure", "Quanta Machine", "Other", 50, "", "", "[]", "[]"},
		{"Freckles Removal", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", "[]", "[]"},
		// Clinic Procedure: CO2 Laser
		{"Single Session", "Clinic Procedure", "CO2 Laser", "Sessions", 200, "", "", "[]", "[]"},
		{"3-Session Package", "Clinic Procedure", "CO2 Laser", "Sessions", 500, "", "", "[]", "[]"},
		{"Under Eyes", "Clinic Procedure", "CO2 Laser", "Face", 150, "", "", "[]", "[]"},
		{"Full Face", "Clinic Procedure", "CO2 Laser", "Face", 300, "", "", "[]", "[]"},
		{"Neck", "Clinic Procedure", "CO2 Laser", "Face", 200, "", "", "[]", "[]"},
		{"Body Area", "Clinic Procedure", "CO2 Laser", "Body", 250, "", "", "[]", "[]"},
		// Clinic Procedure: Creams
		{"Sodermix", "Clinic Procedure", "Creams", "Standard", 30, "", "", "[]", "[]"},
		{"Keloplast", "Clinic Procedure", "Creams", "Standard", 25, "", "", "[]", "[]"},
		{"Beclean", "Clinic Procedure", "Creams", "Standard", 20, "", "", "[]", "[]"},
		{"Boost C", "Clinic Procedure", "Creams", "Standard", 35, "", "", "[]", "[]"},
		{"Boost Eye", "Clinic Procedure", "Creams", "Standard", 30, "", "", "[]", "[]"},
		{"Boost Glow", "Clinic Procedure", "Creams", "Standard", 35, "", "", "[]", "[]"},
		{"Boost Lift", "Clinic Procedure", "Creams", "Standard", 35, "", "", "[]", "[]"},
		{"Boost Mat", "Clinic Procedure", "Creams", "Standard", 30, "", "", "[]", "[]"},
		{"Boost Relax", "Clinic Procedure", "Creams", "Standard", 30, "", "", "[]", "[]"},
		{"Hair Care Serum", "Clinic Procedure", "Creams", "Pharmaceries", 40, "", "", "[]", "[]"},
		{"Retinol Serum", "Clinic Procedure", "Creams", "Pharmaceries", 35, "", "", "[]", "[]"},
		{"Whitening Cream", "Clinic Procedure", "Creams", "Pharmaceries", 30, "", "", "[]", "[]"},
		// Clinic Procedure: Others
		{"Filler Dissolver", "Clinic Procedure", "Others", "General", 150, "", "", "[]", "[]"},
		{"Triple Enzymes", "Clinic Procedure", "Others", "General", 100, "", "", "[]", "[]"},
		{"PRP (Platelet-Rich Plasma)", "Clinic Procedure", "Others", "General", 200, "", "", "[]", "[]"},
		{"Consultation with Dr. Joe", "Clinic Procedure", "Others", "General", 50, "", "", "[]", "[]"},
		// Hospital Surgery
		{"Rhinoplasty", "Hospital Surgery", "Face", "Nose", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Blepharoplasty (Upper)", "Hospital Surgery", "Face", "Eyes", 0, "Consultation required", "Local or general anesthesia", "[]", "[]"},
		{"Blepharoplasty (Lower)", "Hospital Surgery", "Face", "Eyes", 0, "Consultation required", "Local or general anesthesia", "[]", "[]"},
		{"Facelift", "Hospital Surgery", "Face", "Lift", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Neck Lift", "Hospital Surgery", "Face", "Lift", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Otoplasty", "Hospital Surgery", "Face", "Ears", 0, "Consultation required", "Local or general anesthesia", "[]", "[]"},
		{"Liposuction", "Hospital Surgery", "Body", "Contouring", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Abdominoplasty", "Hospital Surgery", "Body", "Contouring", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Brazilian Butt Lift", "Hospital Surgery", "Body", "Contouring", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Breast Augmentation", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Breast Reduction", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Breast Lift", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Gynecomastia", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		// Minor Surgery
		{"Mole Removal", "Minor Surgery", "Skin", "Lesions", 150, "", "Local anesthesia", "[]", "[]"},
		{"Cyst Removal", "Minor Surgery", "Skin", "Lesions", 200, "", "Local anesthesia", "[]", "[]"},
		{"Lipoma Removal", "Minor Surgery", "Skin", "Lesions", 250, "", "Local anesthesia", "[]", "[]"},
		{"Skin Tag Removal", "Minor Surgery", "Skin", "Lesions", 50, "per tag", "", "[]", "[]"},
		{"Wart Removal", "Minor Surgery", "Skin", "Lesions", 50, "per wart", "", "[]", "[]"},
		{"Scar Revision", "Minor Surgery", "Skin", "Reconstruction", 300, "", "Local anesthesia", "[]", "[]"},
		{"Earlobe Repair", "Minor Surgery", "Face", "Reconstruction", 200, "", "Local anesthesia", "[]", "[]"},
		{"Fat Transfer (Face)", "Minor Surgery", "Face", "Enhancement", 0, "Consultation required", "Local anesthesia + sedation", "[]", "[]"},
		{"Thread Lift", "Minor Surgery", "Face", "Enhancement", 0, "Consultation required", "Local anesthesia", "[]", "[]"},
		{"PRP Hair Restoration", "Minor Surgery", "Hair", "Restoration", 250, "per session", "", "[]", "[]"},
	}

	now := "datetime('now')"
	for _, s := range services {
		procID := uuid.Must(uuid.NewV7()).String()
		_, err := db.Exec(
			`INSERT INTO procedures (id, name, procedure_type, category, subcategory, price, price_note, is_active, remarks, includes, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, `+now+`, `+now+`)`,
			procID, s.name, s.procType, s.category, s.subcategory, s.price, s.priceNote, s.remarks, s.includes,
		)
		if err != nil {
			return err
		}

		var sessions []sessionSeed
		if err := json.Unmarshal([]byte(s.sessionPricing), &sessions); err != nil {
			return err
		}
		for i, sess := range sessions {
			sessID := uuid.Must(uuid.NewV7()).String()
			_, err := db.Exec(
				`INSERT INTO procedure_sessions (id, procedure_id, session_number, name, description, price, created_at)
				 VALUES (?, ?, ?, ?, '', ?, `+now+`)`,
				sessID, procID, i+1, sess.Label, sess.Price,
			)
			if err != nil {
				return err
			}
		}
	}

	log.Println("Seeded procedures")
	return nil
}

func seedRooms(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	rooms := []struct{ name, typ string }{
		{"Room 1", "Consultation"},
		{"Room 2", "Procedure"},
		{"Room 3", "General"},
		{"Room 4", "Consultation"},
		{"Room 5", "Procedure"},
		{"Room 6", "General"},
		{"Room 7", "Consultation"},
		{"Hospital", "Hospital"},
	}

	for _, r := range rooms {
		_, err := db.Exec(
			`INSERT INTO rooms (id, name, type, is_available, created_at) VALUES (?, ?, ?, 1, datetime('now'))`,
			uuid.Must(uuid.NewV7()).String(), r.name, r.typ,
		)
		if err != nil {
			return err
		}
	}

	log.Println("Seeded rooms")
	return nil
}

func SeedIfEmpty(db *sql.DB) error {
	if err := seedRoles(db); err != nil {
		return err
	}
	if err := seedProcedures(db); err != nil {
		return err
	}
	if err := seedRooms(db); err != nil {
		return err
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := utils.HashPassword("admin123")
	if err != nil {
		return err
	}

	_, err = db.Exec(
		`INSERT INTO users (id, username, password_hash, display_name, role, is_active)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), "admin", hash, "Test Admin", "super-admin", 1,
	)
	if err != nil {
		return err
	}

	log.Println("Seeded default test user (username: admin, password: admin123)")
	return nil
}
