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

func seedProcedureCategories(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM procedure_categories").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type catSeed struct {
		name, desc string
		children   []struct{ name, desc string }
	}

	categories := []catSeed{
		{"Botox", "Botulinum toxin treatments", []struct{ name, desc string }{
			{"Women", "Botox treatments for women"},
			{"Men", "Botox treatments for men"},
		}},
		{"Face", "Facial procedures", []struct{ name, desc string }{
			{"Face Fillers", "Dermal filler injections for the face"},
			{"Face Skin Boosters", "Skin rejuvenation boosters for the face"},
			{"Face Morpheus8", "Morpheus8 microneedling for the face"},
			{"Face CO2 Laser", "CO2 fractional laser for the face"},
			{"Nose", "Nasal surgeries"},
			{"Eye Surgery", "Eye surgeries"},
			{"Facelift", "Facial lift surgeries"},
			{"Ears", "Ear surgeries"},
			{"Face Reconstruction", "Facial reconstruction procedures"},
			{"Face Enhancement", "Facial enhancement procedures"},
		}},
		{"Body", "Body procedures", []struct{ name, desc string }{
			{"Body Fillers", "Dermal filler injections for the body"},
			{"Body Skin Boosters", "Skin rejuvenation boosters for the body"},
			{"Body Morpheus8", "Morpheus8 microneedling for the body"},
			{"Body CO2 Laser", "CO2 fractional laser for the body"},
			{"Body Contouring", "Body contouring surgeries"},
			{"Breast", "Breast surgeries"},
		}},
		{"Eyes", "Eye area procedures", []struct{ name, desc string }{
			{"Eye Skin Boosters", "Skin rejuvenation boosters for the eye area"},
		}},
		{"Laser Hair Removal", "Laser-based hair removal", []struct{ name, desc string }{
			{"Single Areas", "Individual area treatments"},
			{"Packages", "Multi-area package deals"},
		}},
		{"Quanta Machine", "Quanta laser treatments", []struct{ name, desc string }{
			{"Tattoo Removal", "Laser tattoo removal"},
			{"Varicose", "Varicose vein treatments"},
			{"Melasma", "Melasma laser treatments"},
			{"Other", "Other Quanta laser procedures"},
		}},
		{"CO2 Laser", "CO2 fractional laser treatments", []struct{ name, desc string }{
			{"Sessions", "Session-based CO2 laser packages"},
		}},
		{"Creams", "Topical cream products", []struct{ name, desc string }{
			{"Standard", "Standard clinic creams"},
			{"Pharmaceries", "Pharmacy-grade creams and serums"},
		}},
		{"Others", "Miscellaneous procedures", []struct{ name, desc string }{
			{"General", "General miscellaneous procedures"},
		}},
		{"Skin", "Skin surgical procedures", []struct{ name, desc string }{
			{"Lesions", "Skin lesion removal"},
			{"Skin Reconstruction", "Skin reconstruction procedures"},
		}},
		{"Hair", "Hair restoration procedures", []struct{ name, desc string }{
			{"Restoration", "Hair restoration treatments"},
		}},
	}

	now := "datetime('now')"
	for _, cat := range categories {
		parentID := uuid.Must(uuid.NewV7()).String()
		_, err := db.Exec(
			`INSERT INTO procedure_categories (id, name, description, parent_id, created_at) VALUES (?, ?, ?, '', `+now+`)`,
			parentID, cat.name, cat.desc,
		)
		if err != nil {
			return err
		}
		for _, child := range cat.children {
			_, err := db.Exec(
				`INSERT INTO procedure_categories (id, name, description, parent_id, created_at) VALUES (?, ?, ?, ?, `+now+`)`,
				uuid.Must(uuid.NewV7()).String(), child.name, child.desc, parentID,
			)
			if err != nil {
				return err
			}
		}
	}

	log.Println("Seeded procedure categories")
	return nil
}

func seedProcedureTypes(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM procedure_types").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	types := []struct{ name, desc string }{
		{"Clinic Procedure", "Standard clinic-based procedures"},
		{"Hospital Surgery", "Surgical procedures requiring hospital setting"},
		{"Minor Surgery", "Minor surgical procedures under local anesthesia"},
	}

	now := "datetime('now')"
	for _, t := range types {
		_, err := db.Exec(
			`INSERT INTO procedure_types (id, name, description, created_at) VALUES (?, ?, ?, `+now+`)`,
			uuid.Must(uuid.NewV7()).String(), t.name, t.desc,
		)
		if err != nil {
			return err
		}
	}

	log.Println("Seeded procedure types")
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

	// Build type name to id map
	typeRows, err := db.Query("SELECT id, name FROM procedure_types")
	if err != nil {
		return err
	}
	defer typeRows.Close()
	typeMap := map[string]string{}
	for typeRows.Next() {
		var id, name string
		if err := typeRows.Scan(&id, &name); err != nil {
			return err
		}
		typeMap[name] = id
	}

	// Build subcategory name to id map (subcategories have a parent_id)
	// Key is "parentName/childName" to handle duplicate child names across parents
	catRows, err := db.Query(`
		SELECT c.id, c.name, p.name
		FROM procedure_categories c
		JOIN procedure_categories p ON c.parent_id = p.id
		WHERE c.parent_id != ''`)
	if err != nil {
		return err
	}
	defer catRows.Close()
	catMap := map[string]string{}
	for catRows.Next() {
		var id, name, parentName string
		if err := catRows.Scan(&id, &name, &parentName); err != nil {
			return err
		}
		catMap[parentName+"/"+name] = id
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
		{"Lips", "Clinic Procedure", "Face", "Face Fillers", 250, "", "", "[]", "[]"},
		{"Nasolabial Folds", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", "[]", "[]"},
		{"Marionette Lines", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", "[]", "[]"},
		{"Cheeks", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", "[]", "[]"},
		{"Jawline", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", "[]", "[]"},
		{"Chin", "Clinic Procedure", "Face", "Face Fillers", 300, "", "", "[]", "[]"},
		{"Under Eyes", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", "[]", "[]"},
		{"Nose (Non-Surgical)", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", "[]", "[]"},
		{"Temples", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", "[]", "[]"},
		{"Earlobes", "Clinic Procedure", "Face", "Face Fillers", 200, "", "", "[]", "[]"},
		{"Hands", "Clinic Procedure", "Body", "Body Fillers", 350, "", "", "[]", "[]"},
		// Clinic Procedure: Skin Boosters
		{"Profhilo Face", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", "[]", "[]"},
		{"Profhilo Neck", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", "[]", "[]"},
		{"Skinvive", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", "[]", "[]"},
		{"Sculptra", "Clinic Procedure", "Face", "Face Skin Boosters", 350, "", "", "[]", "[]"},
		{"Exosome", "Clinic Procedure", "Face", "Face Skin Boosters", 350, "", "", "[]", "[]"},
		{"Collagen Booster", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", "[]", "[]"},
		{"Jalupro Face", "Clinic Procedure", "Face", "Face Skin Boosters", 200, "", "", "[]", "[]"},
		{"Jalupro Neck", "Clinic Procedure", "Face", "Face Skin Boosters", 200, "", "", "[]", "[]"},
		{"Profhilo Body", "Clinic Procedure", "Body", "Body Skin Boosters", 350, "", "", "[]", "[]"},
		{"Jalupro Eye", "Clinic Procedure", "Eyes", "Eye Skin Boosters", 200, "", "", "[]", "[]"},
		{"Chroma Phill Art Eye", "Clinic Procedure", "Eyes", "Eye Skin Boosters", 200, "", "", "[]", "[]"},
		// Clinic Procedure: Morpheus8
		{"Face + Plasma", "Clinic Procedure", "Face", "Face Morpheus8", 350, "per session", "", `[{"label":"1 Session","price":350},{"label":"3 Sessions","price":900},{"label":"4 Sessions","price":1100}]`, "[]"},
		{"Neck + Plasma", "Clinic Procedure", "Face", "Face Morpheus8", 250, "per session", "", `[{"label":"1 Session","price":250},{"label":"3 Sessions","price":650}]`, "[]"},
		{"Face & Neck + Plasma", "Clinic Procedure", "Face", "Face Morpheus8", 500, "per session", "", `[{"label":"1 Session","price":500},{"label":"3 Sessions","price":1300}]`, "[]"},
		{"Arms", "Clinic Procedure", "Body", "Body Morpheus8", 350, "per session", "", "[]", "[]"},
		{"Abdominal", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", "[]", "[]"},
		{"Love Handles", "Clinic Procedure", "Body", "Body Morpheus8", 350, "per session", "", "[]", "[]"},
		{"Thighs", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", "[]", "[]"},
		{"Buttocks", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", "[]", "[]"},
		{"Hands", "Clinic Procedure", "Body", "Body Morpheus8", 250, "per session", "", "[]", "[]"},
		{"Knees", "Clinic Procedure", "Body", "Body Morpheus8", 250, "per session", "", "[]", "[]"},
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
		{"Under Eyes", "Clinic Procedure", "Face", "Face CO2 Laser", 150, "", "", "[]", "[]"},
		{"Full Face", "Clinic Procedure", "Face", "Face CO2 Laser", 300, "", "", "[]", "[]"},
		{"Neck", "Clinic Procedure", "Face", "Face CO2 Laser", 200, "", "", "[]", "[]"},
		{"Body Area", "Clinic Procedure", "Body", "Body CO2 Laser", 250, "", "", "[]", "[]"},
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
		{"Blepharoplasty (Upper)", "Hospital Surgery", "Face", "Eye Surgery", 0, "Consultation required", "Local or general anesthesia", "[]", "[]"},
		{"Blepharoplasty (Lower)", "Hospital Surgery", "Face", "Eye Surgery", 0, "Consultation required", "Local or general anesthesia", "[]", "[]"},
		{"Facelift", "Hospital Surgery", "Face", "Facelift", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Neck Lift", "Hospital Surgery", "Face", "Facelift", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Otoplasty", "Hospital Surgery", "Face", "Ears", 0, "Consultation required", "Local or general anesthesia", "[]", "[]"},
		{"Liposuction", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Abdominoplasty", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", "[]", "[]"},
		{"Brazilian Butt Lift", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", "[]", "[]"},
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
		{"Scar Revision", "Minor Surgery", "Skin", "Skin Reconstruction", 300, "", "Local anesthesia", "[]", "[]"},
		{"Earlobe Repair", "Minor Surgery", "Face", "Face Reconstruction", 200, "", "Local anesthesia", "[]", "[]"},
		{"Fat Transfer (Face)", "Minor Surgery", "Face", "Face Enhancement", 0, "Consultation required", "Local anesthesia + sedation", "[]", "[]"},
		{"Thread Lift", "Minor Surgery", "Face", "Face Enhancement", 0, "Consultation required", "Local anesthesia", "[]", "[]"},
		{"PRP Hair Restoration", "Minor Surgery", "Hair", "Restoration", 250, "per session", "", "[]", "[]"},
	}

	now := "datetime('now')"
	for _, s := range services {
		typeID := typeMap[s.procType]
		categoryID := catMap[s.category+"/"+s.subcategory]
		procID := uuid.Must(uuid.NewV7()).String()
		_, err := db.Exec(
			`INSERT INTO procedures (id, name, type_id, category_id, price, price_note, is_active, remarks, includes, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, `+now+`, `+now+`)`,
			procID, s.name, typeID, categoryID, s.price, s.priceNote, s.remarks, s.includes,
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

func seedAllergies(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM allergies").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	allergies := []struct{ name, desc string }{
		// Drug Allergies
		{"Penicillin", "Allergy to penicillin and related antibiotics"},
		{"Amoxicillin", "Allergy to amoxicillin antibiotic"},
		{"Sulfonamides", "Allergy to sulfa drugs"},
		{"Tetracycline", "Allergy to tetracycline antibiotics"},
		{"Cephalosporins", "Allergy to cephalosporin antibiotics"},
		{"Erythromycin", "Allergy to erythromycin antibiotic"},
		{"Ciprofloxacin", "Allergy to fluoroquinolone antibiotics"},
		{"Aspirin", "Allergy to aspirin (acetylsalicylic acid)"},
		{"Ibuprofen", "Allergy to ibuprofen and NSAIDs"},
		{"Naproxen", "Allergy to naproxen"},
		{"Codeine", "Allergy to codeine and related opioids"},
		{"Morphine", "Allergy to morphine"},
		{"Tramadol", "Allergy to tramadol"},
		{"Lidocaine", "Allergy to lidocaine local anesthetic"},
		{"Novocaine", "Allergy to novocaine (procaine)"},
		{"Benzocaine", "Allergy to benzocaine topical anesthetic"},
		{"Tetracaine", "Allergy to tetracaine anesthetic"},
		{"Bupivacaine", "Allergy to bupivacaine anesthetic"},
		{"Insulin", "Allergy to insulin preparations"},
		{"Metformin", "Allergy to metformin"},
		{"Phenytoin", "Allergy to phenytoin anticonvulsant"},
		{"Carbamazepine", "Allergy to carbamazepine"},
		{"Methotrexate", "Allergy to methotrexate"},
		{"Heparin", "Allergy to heparin anticoagulant"},
		{"Warfarin", "Allergy to warfarin anticoagulant"},
		// Anesthetic Allergies
		{"General Anesthesia", "Adverse reactions to general anesthetics"},
		{"Propofol", "Allergy to propofol (may cross-react with egg/soy)"},
		{"Sevoflurane", "Allergy to sevoflurane inhalation anesthetic"},
		// Food Allergies
		{"Peanuts", "Allergy to peanuts"},
		{"Tree Nuts", "Allergy to tree nuts (almonds, cashews, walnuts, etc.)"},
		{"Milk", "Allergy to cow's milk proteins"},
		{"Eggs", "Allergy to chicken eggs"},
		{"Wheat", "Allergy to wheat (distinct from celiac disease)"},
		{"Soy", "Allergy to soybeans and soy products"},
		{"Fish", "Allergy to fish"},
		{"Shellfish", "Allergy to shellfish (shrimp, crab, lobster)"},
		{"Sesame", "Allergy to sesame seeds"},
		{"Corn", "Allergy to corn and corn-derived products"},
		{"Gluten", "Gluten intolerance or celiac disease"},
		{"Mustard", "Allergy to mustard"},
		{"Celery", "Allergy to celery"},
		{"Lupin", "Allergy to lupin beans"},
		{"Mollusks", "Allergy to mollusks (clams, mussels, oysters)"},
		// Environmental Allergies
		{"Dust Mites", "Allergy to house dust mites"},
		{"Pollen", "Allergy to pollen (hay fever)"},
		{"Mold", "Allergy to mold spores"},
		{"Pet Dander (Cat)", "Allergy to cat dander"},
		{"Pet Dander (Dog)", "Allergy to dog dander"},
		{"Cockroach", "Allergy to cockroach droppings"},
		{"Grass", "Allergy to grass pollen"},
		{"Ragweed", "Allergy to ragweed pollen"},
		// Insect Allergies
		{"Bee Venom", "Allergy to bee stings"},
		{"Wasp Venom", "Allergy to wasp stings"},
		{"Fire Ant Venom", "Allergy to fire ant stings"},
		{"Mosquito Bites", "Severe allergy to mosquito bites"},
		// Contact Allergies
		{"Latex", "Allergy to natural rubber latex"},
		{"Nickel", "Allergy to nickel (common in jewelry and metals)"},
		{"Cobalt", "Allergy to cobalt"},
		{"Chromium", "Allergy to chromium"},
		{"Fragrance Mix", "Allergy to fragrance compounds"},
		{"Formaldehyde", "Allergy to formaldehyde and releasers"},
		{"Lanolin", "Allergy to lanolin (wool alcohols)"},
		{"Neomycin", "Contact allergy to neomycin antibiotic"},
		{"Bacitracin", "Contact allergy to bacitracin antibiotic"},
		{"Adhesive Tape", "Allergy to medical adhesive tape"},
		{"Hair Dye (PPD)", "Allergy to paraphenylenediamine in hair dyes"},
		// Cosmetic & Dermal Filler Allergies
		{"Hyaluronic Acid", "Allergy to hyaluronic acid dermal fillers"},
		{"Botulinum Toxin", "Allergy to botulinum toxin (Botox)"},
		{"Poly-L-Lactic Acid", "Allergy to Sculptra (poly-L-lactic acid)"},
		{"Calcium Hydroxylapatite", "Allergy to Radiesse filler"},
		{"Collagen (Bovine)", "Allergy to bovine collagen"},
		{"Collagen (Human)", "Allergy to human-derived collagen"},
		{"Silicone", "Allergy to silicone implants or injections"},
		{"Retinoids", "Allergy to retinol / retinoid compounds"},
		{"Glycolic Acid", "Allergy to glycolic acid (chemical peels)"},
		{"Salicylic Acid", "Allergy to salicylic acid"},
		{"Hydroquinone", "Allergy to hydroquinone skin lightener"},
		{"Vitamin C (Topical)", "Allergy to topical ascorbic acid"},
		{"Sunscreen (Chemical)", "Allergy to chemical sunscreen agents"},
		{"Parabens", "Allergy to paraben preservatives"},
		{"Propylene Glycol", "Allergy to propylene glycol"},
		// Contrast & Medical Material Allergies
		{"Iodine Contrast Dye", "Allergy to iodinated contrast media"},
		{"Gadolinium", "Allergy to gadolinium MRI contrast"},
		{"Chlorhexidine", "Allergy to chlorhexidine antiseptic"},
		{"Povidone-Iodine", "Allergy to betadine / povidone-iodine"},
		{"Surgical Sutures", "Allergy to suture materials"},
		{"Titanium", "Allergy to titanium implants or devices"},
		{"Stainless Steel", "Allergy to stainless steel (surgical instruments)"},
		// Other
		{"Alcohol (Ethanol)", "Allergy or intolerance to ethanol"},
		{"Preservatives (Thimerosal)", "Allergy to thimerosal in vaccines"},
		{"Dyes (Tartrazine)", "Allergy to tartrazine yellow dye"},
		{"Epinephrine", "Rare allergy to epinephrine"},
		{"Aloe Vera", "Allergy to aloe vera"},
		{"Tea Tree Oil", "Allergy to tea tree oil"},
		{"Chamomile", "Allergy to chamomile"},
		{"Eucalyptus", "Allergy to eucalyptus oil"},
		{"Cinnamon", "Allergy to cinnamon (contact or ingestion)"},
	}

	now := "datetime('now')"
	for _, a := range allergies {
		_, err := db.Exec(
			`INSERT INTO allergies (id, name, description, created_at) VALUES (?, ?, ?, `+now+`)`,
			uuid.Must(uuid.NewV7()).String(), a.name, a.desc,
		)
		if err != nil {
			return err
		}
	}

	log.Println("Seeded allergies")
	return nil
}

func seedCurrencies(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM currencies").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	_, err := db.Exec(
		`INSERT INTO currencies (id, code, name, symbol, exchange_rate, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
		uuid.Must(uuid.NewV7()).String(), "USD", "US Dollar", "$", 1.0,
	)
	if err != nil {
		return err
	}

	log.Println("Seeded default currencies")
	return nil
}

func seedSelfBalance(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM balances WHERE entity_type = 'self'").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	var currencyID string
	if err := db.QueryRow("SELECT id FROM currencies WHERE code = 'USD'").Scan(&currencyID); err != nil {
		return err
	}

	if _, err := db.Exec(
		`INSERT OR IGNORE INTO balances (id, entity_type, entity_id, entity_name, currency_id, amount, created_at, updated_at)
		 VALUES (?, 'self', 'self', 'Clinic', ?, 0, datetime('now'), datetime('now'))`,
		uuid.Must(uuid.NewV7()).String(), currencyID,
	); err != nil {
		return err
	}

	log.Println("Seeded self (clinic) balance")
	return nil
}

func seedProductCategories(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM product_categories").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type catSeed struct {
		name, desc string
		children   []struct{ name, desc string }
	}

	categories := []catSeed{
		{"Injectables", "Injectable products for aesthetic procedures", []struct{ name, desc string }{
			{"Botulinum Toxins", "Botox, Dysport, and other neuromodulators"},
			{"Dermal Fillers", "Hyaluronic acid and other filler products"},
			{"Skin Boosters", "Profhilo, Skinvive, and similar products"},
			{"Biostimulators", "Sculptra, Radiesse, and collagen stimulators"},
			{"Dissolvers", "Hyaluronidase and filler dissolving agents"},
		}},
		{"Topical Products", "Products applied to the skin surface", []struct{ name, desc string }{
			{"Creams & Moisturizers", "Hydrating and therapeutic creams"},
			{"Serums", "Concentrated treatment serums"},
			{"Sunscreens", "Sun protection products"},
			{"Cleansers", "Facial and skin cleansers"},
			{"Exfoliants", "Chemical and physical exfoliating products"},
			{"Masks", "Treatment masks and peels"},
			{"Eye Care", "Under-eye and eyelid treatments"},
			{"Lip Care", "Lip balms and treatments"},
		}},
		{"Anesthetics", "Local and topical anesthetics", []struct{ name, desc string }{
			{"Topical Anesthetics", "Numbing creams and gels"},
			{"Injectable Anesthetics", "Lidocaine, bupivacaine, and similar"},
		}},
		{"Surgical Supplies", "Materials used in surgical procedures", []struct{ name, desc string }{
			{"Sutures", "Surgical sutures and threads"},
			{"Dressings & Bandages", "Wound care and post-procedure dressings"},
			{"Gloves", "Surgical and examination gloves"},
			{"Drapes & Gowns", "Sterile drapes and surgical gowns"},
			{"Scalpels & Blades", "Cutting instruments"},
			{"Cannulas & Needles", "Injection and infusion cannulas"},
		}},
		{"Antiseptics & Disinfectants", "Cleaning and sterilization products", []struct{ name, desc string }{
			{"Skin Antiseptics", "Chlorhexidine, betadine, alcohol prep"},
			{"Surface Disinfectants", "Equipment and surface cleaners"},
			{"Hand Sanitizers", "Hand hygiene products"},
		}},
		{"Laser & Device Consumables", "Consumables for laser and energy devices", []struct{ name, desc string }{
			{"Laser Tips & Handpieces", "Replacement tips for laser machines"},
			{"Cooling Gels", "Conductive and cooling gels for devices"},
			{"Microneedling Cartridges", "Replacement cartridges for microneedling"},
			{"IPL Filters", "Filters and accessories for IPL devices"},
		}},
		{"PRP & Regenerative", "Platelet-rich plasma and regenerative products", []struct{ name, desc string }{
			{"PRP Kits", "Blood collection and centrifuge kits"},
			{"Exosomes", "Exosome therapy products"},
			{"Growth Factors", "Topical and injectable growth factors"},
		}},
		{"Pharmaceuticals", "Prescription and OTC medications", []struct{ name, desc string }{
			{"Antibiotics", "Oral and topical antibiotics"},
			{"Anti-Inflammatories", "NSAIDs and corticosteroids"},
			{"Analgesics", "Pain relief medications"},
			{"Antihistamines", "Allergy medications"},
		}},
		{"Hair Care", "Hair treatment and restoration products", []struct{ name, desc string }{
			{"Hair Growth Treatments", "Minoxidil, finasteride, and similar"},
			{"Scalp Treatments", "Shampoos and scalp serums"},
		}},
		{"Consultation Supplies", "Items used during patient consultations", []struct{ name, desc string }{
			{"Consent Forms", "Printed consent and intake forms"},
			{"Measurement Tools", "Calipers, rulers, and assessment tools"},
			{"Photography Supplies", "Backdrops, lighting for clinical photos"},
		}},
	}

	now := "datetime('now')"
	for _, cat := range categories {
		parentID := uuid.Must(uuid.NewV7()).String()
		_, err := db.Exec(
			`INSERT INTO product_categories (id, name, description, parent_id, created_at) VALUES (?, ?, ?, '', `+now+`)`,
			parentID, cat.name, cat.desc,
		)
		if err != nil {
			return err
		}
		for _, child := range cat.children {
			_, err := db.Exec(
				`INSERT INTO product_categories (id, name, description, parent_id, created_at) VALUES (?, ?, ?, ?, `+now+`)`,
				uuid.Must(uuid.NewV7()).String(), child.name, child.desc, parentID,
			)
			if err != nil {
				return err
			}
		}
	}

	log.Println("Seeded product categories")
	return nil
}

func seedProducts(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM products").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// Build subcategory name to id map
	rows, err := db.Query("SELECT id, name FROM product_categories WHERE parent_id != ''")
	if err != nil {
		return err
	}
	defer rows.Close()
	catMap := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		catMap[name] = id
	}

	type prodSeed struct {
		name     string
		category string
		qty      int
		minQty   int
		price    float64
	}

	products := []prodSeed{
		// Botulinum Toxins
		{"Botox (Allergan) 50U", "Botulinum Toxins", 30, 5, 150},
		{"Botox (Allergan) 100U", "Botulinum Toxins", 40, 10, 280},
		{"Botox (Allergan) 200U", "Botulinum Toxins", 15, 3, 520},
		{"Dysport 300U", "Botulinum Toxins", 25, 5, 200},
		{"Dysport 500U", "Botulinum Toxins", 15, 3, 320},
		{"Xeomin 50U", "Botulinum Toxins", 10, 2, 160},
		{"Xeomin 100U", "Botulinum Toxins", 10, 2, 290},
		// Dermal Fillers
		{"Juvederm Ultra XC 1ml", "Dermal Fillers", 30, 5, 250},
		{"Juvederm Voluma XC 1ml", "Dermal Fillers", 20, 5, 320},
		{"Juvederm Volbella XC 1ml", "Dermal Fillers", 20, 5, 280},
		{"Juvederm Vollure XC 1ml", "Dermal Fillers", 15, 3, 300},
		{"Restylane 1ml", "Dermal Fillers", 25, 5, 240},
		{"Restylane Lyft 1ml", "Dermal Fillers", 15, 3, 280},
		{"Restylane Kysse 1ml", "Dermal Fillers", 15, 3, 270},
		{"Belotero Balance 1ml", "Dermal Fillers", 10, 2, 230},
		{"Radiesse 1.5ml", "Dermal Fillers", 10, 2, 300},
		// Skin Boosters
		{"Profhilo 2ml", "Skin Boosters", 20, 5, 180},
		{"Skinvive 1ml", "Skin Boosters", 15, 3, 200},
		{"Jalupro Classic", "Skin Boosters", 15, 3, 120},
		{"Jalupro HMW", "Skin Boosters", 10, 2, 150},
		// Biostimulators
		{"Sculptra (PLLA) Vial", "Biostimulators", 10, 2, 350},
		{"Radiesse (+) 1.5ml", "Biostimulators", 8, 2, 320},
		// Dissolvers
		{"Hyaluronidase (Hylenex) 150U", "Dissolvers", 10, 3, 80},
		// Creams & Moisturizers
		{"Sodermix Cream", "Creams & Moisturizers", 25, 5, 30},
		{"Keloplast Cream", "Creams & Moisturizers", 25, 5, 25},
		{"Boost Lift Cream", "Creams & Moisturizers", 20, 5, 35},
		{"Boost Relax Cream", "Creams & Moisturizers", 20, 5, 30},
		{"Boost Mat Cream", "Creams & Moisturizers", 20, 5, 30},
		{"CeraVe Moisturizing Cream", "Creams & Moisturizers", 30, 10, 18},
		{"La Roche-Posay Cicaplast Baume B5", "Creams & Moisturizers", 20, 5, 22},
		{"Hydroquinone 4% Cream", "Creams & Moisturizers", 10, 3, 35},
		// Serums
		{"Boost C Serum", "Serums", 20, 5, 35},
		{"Boost Glow Serum", "Serums", 20, 5, 35},
		{"Retinol Serum 0.5%", "Serums", 15, 5, 35},
		{"Retinol Serum 1%", "Serums", 15, 5, 40},
		{"Hair Care Serum", "Serums", 20, 5, 40},
		{"Niacinamide 10% Serum", "Serums", 15, 5, 25},
		{"Hyaluronic Acid Serum", "Serums", 20, 5, 28},
		// Sunscreens
		{"SPF 50+ Sunscreen 50ml", "Sunscreens", 40, 10, 22},
		{"Tinted SPF 50+ Sunscreen 50ml", "Sunscreens", 30, 10, 28},
		{"SPF 30 Lip Balm", "Sunscreens", 25, 5, 10},
		// Cleansers
		{"Gentle Foaming Cleanser 200ml", "Cleansers", 25, 5, 18},
		{"Micellar Water 400ml", "Cleansers", 20, 5, 15},
		{"Beclean Cleanser", "Cleansers", 20, 5, 20},
		// Exfoliants
		{"Glycolic Acid Peel 30%", "Exfoliants", 10, 3, 45},
		{"Glycolic Acid Peel 50%", "Exfoliants", 8, 2, 55},
		{"Glycolic Acid Peel 70%", "Exfoliants", 5, 2, 65},
		{"Salicylic Acid Peel 20%", "Exfoliants", 10, 3, 40},
		{"TCA Peel 15%", "Exfoliants", 8, 2, 50},
		// Masks
		{"Hydrating Sheet Mask (10-pack)", "Masks", 15, 5, 25},
		{"Charcoal Detox Mask 100ml", "Masks", 10, 3, 20},
		{"Vitamin C Brightening Mask 100ml", "Masks", 10, 3, 22},
		// Eye Care
		{"Boost Eye Cream", "Eye Care", 20, 5, 30},
		{"Chroma Phill Art Eye", "Eye Care", 10, 3, 200},
		{"Eye Contour Serum 15ml", "Eye Care", 15, 5, 32},
		// Lip Care
		{"Lip Repair Balm", "Lip Care", 20, 5, 12},
		{"Lip Plumping Serum", "Lip Care", 15, 5, 18},
		// Topical Anesthetics
		{"EMLA Cream 5% 30g", "Topical Anesthetics", 30, 10, 25},
		{"LMX 4% Cream 15g", "Topical Anesthetics", 25, 5, 20},
		{"BLT Compound Cream 30g", "Topical Anesthetics", 20, 5, 35},
		// Injectable Anesthetics
		{"Lidocaine 1% 20ml", "Injectable Anesthetics", 40, 10, 8},
		{"Lidocaine 2% 20ml", "Injectable Anesthetics", 40, 10, 10},
		{"Lidocaine 2% with Epinephrine 20ml", "Injectable Anesthetics", 30, 10, 12},
		{"Bupivacaine 0.25% 20ml", "Injectable Anesthetics", 15, 5, 15},
		// Sutures
		{"PDO Threads Mono (10-pack)", "Sutures", 20, 5, 60},
		{"PDO Threads Cog (10-pack)", "Sutures", 15, 3, 120},
		{"Vicryl 4-0 Suture", "Sutures", 30, 10, 8},
		{"Nylon 5-0 Suture", "Sutures", 30, 10, 6},
		// Dressings & Bandages
		{"Sterile Gauze Pads 4x4 (100-pack)", "Dressings & Bandages", 20, 5, 12},
		{"Adhesive Bandages (100-pack)", "Dressings & Bandages", 15, 5, 10},
		{"Steri-Strips (50-pack)", "Dressings & Bandages", 15, 5, 18},
		{"Tegaderm Film (50-pack)", "Dressings & Bandages", 10, 3, 35},
		// Gloves
		{"Nitrile Gloves Small (100-pack)", "Gloves", 15, 5, 12},
		{"Nitrile Gloves Medium (100-pack)", "Gloves", 20, 5, 12},
		{"Nitrile Gloves Large (100-pack)", "Gloves", 15, 5, 12},
		// Drapes & Gowns
		{"Disposable Patient Gowns (25-pack)", "Drapes & Gowns", 10, 3, 30},
		{"Sterile Drape Sheets (10-pack)", "Drapes & Gowns", 10, 3, 25},
		// Scalpels & Blades
		{"Scalpel Blade #11 (10-pack)", "Scalpels & Blades", 15, 5, 15},
		{"Scalpel Blade #15 (10-pack)", "Scalpels & Blades", 15, 5, 15},
		{"Disposable Biopsy Punch 3mm (10-pack)", "Scalpels & Blades", 10, 3, 20},
		// Cannulas & Needles
		{"25G x 50mm Cannula (20-pack)", "Cannulas & Needles", 20, 5, 45},
		{"27G x 38mm Cannula (20-pack)", "Cannulas & Needles", 20, 5, 40},
		{"30G x 13mm Needle (100-pack)", "Cannulas & Needles", 25, 5, 18},
		{"32G x 4mm Needle (100-pack)", "Cannulas & Needles", 25, 5, 22},
		{"18G Drawing Needle (100-pack)", "Cannulas & Needles", 15, 5, 12},
		// Skin Antiseptics
		{"Chlorhexidine 2% Solution 500ml", "Skin Antiseptics", 15, 5, 12},
		{"Betadine Solution 500ml", "Skin Antiseptics", 15, 5, 10},
		{"Alcohol Prep Pads (200-pack)", "Skin Antiseptics", 20, 5, 8},
		// Surface Disinfectants
		{"CaviCide Surface Disinfectant 1L", "Surface Disinfectants", 10, 3, 18},
		{"Autoclave Cleaning Solution 500ml", "Surface Disinfectants", 8, 2, 22},
		// Hand Sanitizers
		{"Hand Sanitizer Gel 500ml", "Hand Sanitizers", 20, 5, 8},
		{"Surgical Hand Scrub 500ml", "Hand Sanitizers", 10, 3, 15},
		// Laser Tips & Handpieces
		{"Morpheus8 Tip 24-pin", "Laser Tips & Handpieces", 15, 3, 85},
		{"Morpheus8 Tip 40-pin", "Laser Tips & Handpieces", 15, 3, 95},
		// Cooling Gels
		{"Ultrasound Gel 250ml", "Cooling Gels", 20, 5, 8},
		{"Aloe Cooling Gel 200ml", "Cooling Gels", 15, 5, 12},
		// Microneedling Cartridges
		{"Dermapen Cartridge 16-pin (10-pack)", "Microneedling Cartridges", 15, 3, 50},
		{"Dermapen Cartridge 36-pin (10-pack)", "Microneedling Cartridges", 15, 3, 55},
		// IPL Filters
		{"IPL 515nm Filter", "IPL Filters", 3, 1, 120},
		{"IPL 560nm Filter", "IPL Filters", 3, 1, 120},
		// PRP Kits
		{"PRP Collection Kit", "PRP Kits", 15, 5, 45},
		{"PRP Centrifuge Tubes (10-pack)", "PRP Kits", 10, 3, 60},
		// Exosomes
		{"Exosome Vial 5ml", "Exosomes", 8, 2, 250},
		// Growth Factors
		{"EGF Serum 30ml", "Growth Factors", 10, 3, 80},
		{"PDRN Salmon DNA 3ml", "Growth Factors", 10, 3, 65},
		// Antibiotics
		{"Amoxicillin 500mg (30 caps)", "Antibiotics", 15, 5, 12},
		{"Cephalexin 500mg (30 caps)", "Antibiotics", 10, 3, 15},
		{"Mupirocin Ointment 15g", "Antibiotics", 20, 5, 18},
		{"Fusidic Acid Cream 15g", "Antibiotics", 15, 5, 16},
		// Anti-Inflammatories
		{"Dexamethasone 4mg/ml (10 vials)", "Anti-Inflammatories", 10, 3, 30},
		{"Triamcinolone 40mg/ml Vial", "Anti-Inflammatories", 10, 3, 25},
		{"Ibuprofen 400mg (30 tabs)", "Anti-Inflammatories", 15, 5, 8},
		// Analgesics
		{"Paracetamol 500mg (30 tabs)", "Analgesics", 20, 5, 5},
		{"Tramadol 50mg (20 caps)", "Analgesics", 10, 3, 15},
		// Antihistamines
		{"Cetirizine 10mg (30 tabs)", "Antihistamines", 15, 5, 6},
		{"Diphenhydramine 25mg (30 caps)", "Antihistamines", 10, 3, 7},
		// Hair Growth Treatments
		{"Minoxidil 5% Solution 60ml", "Hair Growth Treatments", 15, 5, 25},
		{"Finasteride 1mg (30 tabs)", "Hair Growth Treatments", 10, 3, 20},
		// Scalp Treatments
		{"Ketoconazole Shampoo 2% 120ml", "Scalp Treatments", 10, 3, 18},
		{"Scalp Revitalizing Serum 50ml", "Scalp Treatments", 10, 3, 30},
	}

	now := "datetime('now')"
	for _, p := range products {
		catID := catMap[p.category]
		_, err := db.Exec(
			`INSERT INTO products (id, name, category_id, quantity, min_threshold, unit_price, created_at) VALUES (?, ?, ?, ?, ?, ?, `+now+`)`,
			uuid.Must(uuid.NewV7()).String(), p.name, catID, p.qty, p.minQty, p.price,
		)
		if err != nil {
			return err
		}
	}

	log.Println("Seeded products")
	return nil
}

func seedSuppliers(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM suppliers").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	suppliers := []struct {
		name, contact, email, address string
	}{
		{"MedSupply Co.", "+961 1 234 567", "orders@example.com", "Beirut, Lebanon"},
		{"DermaPharma", "+961 1 345 678", "info@example.org", "Jounieh, Lebanon"},
		{"Aesthetic Essentials", "+961 3 456 789", "sales@example.net", "Tripoli, Lebanon"},
		{"BioTech Materials", "+961 1 567 890", "supply@example.net", "Sidon, Lebanon"},
	}

	for _, s := range suppliers {
		_, err := db.Exec(
			`INSERT INTO suppliers (id, name, contact, email, address, created_at, updated_at) VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))`,
			uuid.Must(uuid.NewV7()).String(), s.name, s.contact, s.email, s.address,
		)
		if err != nil {
			return err
		}
	}

	log.Println("Seeded suppliers")
	return nil
}

func SeedIfEmpty(db *sql.DB) error {
	if err := seedRoles(db); err != nil {
		return err
	}
	if err := seedProcedureTypes(db); err != nil {
		return err
	}
	if err := seedProcedureCategories(db); err != nil {
		return err
	}
	if err := seedProcedures(db); err != nil {
		return err
	}
	if err := seedRooms(db); err != nil {
		return err
	}
	if err := seedCurrencies(db); err != nil {
		return err
	}
	if err := seedSelfBalance(db); err != nil {
		return err
	}
	if err := seedAllergies(db); err != nil {
		return err
	}
	if err := seedProductCategories(db); err != nil {
		return err
	}
	if err := seedProducts(db); err != nil {
		return err
	}
	if err := seedSuppliers(db); err != nil {
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
