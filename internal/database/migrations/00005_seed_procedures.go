package migrations

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedProcedures, downSeedProcedures)
}

func upSeedProcedures(ctx context.Context, tx *sql.Tx) error {
	typeRows, err := tx.QueryContext(ctx, "SELECT id, name FROM procedure_types")
	if err != nil {
		return err
	}
	typeMap := map[string]string{}
	for typeRows.Next() {
		var id, name string
		if err := typeRows.Scan(&id, &name); err != nil {
			typeRows.Close()
			return err
		}
		typeMap[name] = id
	}
	typeRows.Close()

	catRows, err := tx.QueryContext(ctx, `
		SELECT c.id, c.name, p.name
		FROM procedure_categories c
		JOIN procedure_categories p ON c.parent_id = p.id
		WHERE c.parent_id != ''`)
	if err != nil {
		return err
	}
	catMap := map[string]string{}
	for catRows.Next() {
		var id, name, parentName string
		if err := catRows.Scan(&id, &name, &parentName); err != nil {
			catRows.Close()
			return err
		}
		catMap[parentName+"/"+name] = id
	}
	catRows.Close()

	type svcSeed struct {
		name, procType, category, subcategory string
		price                                 float64
		priceNote, remarks, includes          string
	}

	services := []svcSeed{
		// --- Clinic Procedure - Botox (Women) ---
		{"Botox Full", "Clinic Procedure", "Botox", "Women", 220, "", "", ""},
		{"Botox Full (Dysport)", "Clinic Procedure", "Botox", "Women", 250, "", "", ""},
		{"Botox Full + Bunny Lines", "Clinic Procedure", "Botox", "Women", 250, "", "", ""},
		{"Botox Marionette Lines (DAO)", "Clinic Procedure", "Botox", "Women", 30, "", "", ""},
		{"Botox Around Eyes", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		{"Botox Frown Lines", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		{"Botox Gummy Smile", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		{"Botox Clenching Teeth", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"Botox Sweating", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"Botox Migraine", "Clinic Procedure", "Botox", "Women", 400, "", "", ""},
		{"Botox Neck", "Clinic Procedure", "Botox", "Women", 220, "", "", ""},
		{"Botox Calves", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"Botox Jaw", "Clinic Procedure", "Botox", "Women", 150, "", "", ""},
		{"Traptox", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"Lip Flip", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		// --- Clinic Procedure - Botox (Men) ---
		{"Botox Full", "Clinic Procedure", "Botox", "Men", 250, "", "", ""},
		{"Botox Full (Dysport)", "Clinic Procedure", "Botox", "Men", 280, "", "", ""},
		{"Botox Full + Bunny Lines", "Clinic Procedure", "Botox", "Men", 280, "", "", ""},
		{"Botox Marionette Lines (DAO)", "Clinic Procedure", "Botox", "Men", 30, "", "", ""},
		{"Botox Around Eyes", "Clinic Procedure", "Botox", "Men", 150, "", "", ""},
		{"Botox Frown Lines", "Clinic Procedure", "Botox", "Men", 150, "", "", ""},
		{"Botox Gummy Smile", "Clinic Procedure", "Botox", "Men", 120, "", "", ""},
		{"Botox Teeth Clenching", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"Botox Sweating", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"Botox Migraine", "Clinic Procedure", "Botox", "Men", 400, "", "", ""},
		{"Botox Neck", "Clinic Procedure", "Botox", "Men", 250, "", "", ""},
		{"Botox Calves", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"Botox Jaw", "Clinic Procedure", "Botox", "Men", 180, "", "", ""},
		{"Traptox", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"Lip Flip", "Clinic Procedure", "Botox", "Men", 120, "", "", ""},
		// --- Clinic Procedure - Fillers ---
		{"Lips", "Clinic Procedure", "Face", "Face Fillers", 250, "", "", ""},
		{"Nasolabial Folds", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"Marionette Lines", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"Cheeks", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"Jawline", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"Chin", "Clinic Procedure", "Face", "Face Fillers", 300, "", "", ""},
		{"Under Eyes", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"Nose (Non-Surgical)", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"Temples", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"Earlobes", "Clinic Procedure", "Face", "Face Fillers", 200, "", "", ""},
		{"Hands", "Clinic Procedure", "Body", "Body Fillers", 350, "", "", ""},
		// --- Clinic Procedure - Skin Boosters ---
		{"Profhilo Face", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"Profhilo Neck", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"Skinvive", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"Sculptra", "Clinic Procedure", "Face", "Face Skin Boosters", 350, "", "", ""},
		{"Exosome", "Clinic Procedure", "Face", "Face Skin Boosters", 350, "", "", ""},
		{"Collagen Booster", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"Jalupro Face", "Clinic Procedure", "Face", "Face Skin Boosters", 200, "", "", ""},
		{"Jalupro Neck", "Clinic Procedure", "Face", "Face Skin Boosters", 200, "", "", ""},
		{"Profhilo Body", "Clinic Procedure", "Body", "Body Skin Boosters", 350, "", "", ""},
		{"Jalupro Eye", "Clinic Procedure", "Eyes", "Eye Skin Boosters", 200, "", "", ""},
		{"Chroma Phill Art Eye", "Clinic Procedure", "Eyes", "Eye Skin Boosters", 200, "", "", ""},
		// --- Clinic Procedure - Morpheus8 ---
		{"Face + Plasma (1 session)", "Clinic Procedure", "Face", "Face Morpheus8", 350, "per session", "", ""},
		{"Face + Plasma (3 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 900, "", "", ""},
		{"Face + Plasma (4 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 1100, "", "", ""},
		{"Neck + Plasma (1 session)", "Clinic Procedure", "Face", "Face Morpheus8", 250, "per session", "", ""},
		{"Neck + Plasma (3 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 650, "", "", ""},
		{"Face & Neck + Plasma (1 session)", "Clinic Procedure", "Face", "Face Morpheus8", 500, "per session", "", ""},
		{"Face & Neck + Plasma (3 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 1300, "", "", ""},
		{"Arms", "Clinic Procedure", "Body", "Body Morpheus8", 350, "per session", "", ""},
		{"Abdominal", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", ""},
		{"Love Handles", "Clinic Procedure", "Body", "Body Morpheus8", 350, "per session", "", ""},
		{"Thighs", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", ""},
		{"Buttocks", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", ""},
		{"Hands", "Clinic Procedure", "Body", "Body Morpheus8", 250, "per session", "", ""},
		{"Knees", "Clinic Procedure", "Body", "Body Morpheus8", 250, "per session", "", ""},
		// --- Clinic Procedure - Laser Hair Removal ---
		{"Full Face", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"Upper Lip", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 15, "", "", ""},
		{"Sideburns", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 20, "", "", ""},
		{"Chin", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 15, "", "", ""},
		{"Full Arms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"Half Arms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"Underarms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 25, "", "", ""},
		{"Full Legs", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 80, "", "", ""},
		{"Half Legs", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", ""},
		{"Bikini", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"Brazilian", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"Chest", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", ""},
		{"Abdomen", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", ""},
		{"Full Back", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"Half Back", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"Neck", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 25, "", "", ""},
		{"Buttocks", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"Hands / Feet", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 20, "", "", ""},
		{"Full Body Package 1", "Clinic Procedure", "Laser Hair Removal", "Packages", 200, "", "", "Full Face, Underarms, Full Arms, Brazilian"},
		{"Full Body Package 2", "Clinic Procedure", "Laser Hair Removal", "Packages", 250, "", "", "Full Face, Underarms, Full Arms, Brazilian, Full Legs"},
		{"Full Body Package 3", "Clinic Procedure", "Laser Hair Removal", "Packages", 300, "", "", "Full Face, Underarms, Full Arms, Brazilian, Full Legs, Full Back"},
		// --- Clinic Procedure - Quanta Machine ---
		{"Tattoo Removal - Eyebrows", "Clinic Procedure", "Quanta Machine", "Tattoo Removal", 100, "", "", ""},
		{"Tattoo Removal - Face/Body", "Clinic Procedure", "Quanta Machine", "Tattoo Removal", 150, "", "", ""},
		{"Varicose Vein - Per Vein", "Clinic Procedure", "Quanta Machine", "Varicose", 50, "per vein", "", ""},
		{"Varicose - Full Face", "Clinic Procedure", "Quanta Machine", "Varicose", 150, "", "", ""},
		{"Varicose - Full Body", "Clinic Procedure", "Quanta Machine", "Varicose", 300, "", "", ""},
		{"Melasma Q-Switched", "Clinic Procedure", "Quanta Machine", "Melasma", 100, "", "", ""},
		{"Melasma Full Face", "Clinic Procedure", "Quanta Machine", "Melasma", 200, "", "", ""},
		{"Carbon Peel", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		{"Hair Bleaching", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		{"Rosacea Treatment", "Clinic Procedure", "Quanta Machine", "Other", 150, "", "", ""},
		{"Scar Treatment", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		{"Cherry Angiomas", "Clinic Procedure", "Quanta Machine", "Other", 50, "", "", ""},
		{"Freckles Removal", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		// --- Clinic Procedure - CO2 Laser ---
		{"Single Session", "Clinic Procedure", "CO2 Laser", "Sessions", 200, "", "", ""},
		{"3-Session Package", "Clinic Procedure", "CO2 Laser", "Sessions", 500, "", "", ""},
		{"Under Eyes", "Clinic Procedure", "Face", "Face CO2 Laser", 150, "", "", ""},
		{"Full Face", "Clinic Procedure", "Face", "Face CO2 Laser", 300, "", "", ""},
		{"Neck", "Clinic Procedure", "Face", "Face CO2 Laser", 200, "", "", ""},
		{"Body Area", "Clinic Procedure", "Body", "Body CO2 Laser", 250, "", "", ""},
		// --- Clinic Procedure - Creams ---
		{"Sodermix", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"Keloplast", "Clinic Procedure", "Creams", "Standard", 25, "", "", ""},
		{"Beclean", "Clinic Procedure", "Creams", "Standard", 20, "", "", ""},
		{"Boost C", "Clinic Procedure", "Creams", "Standard", 35, "", "", ""},
		{"Boost Eye", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"Boost Glow", "Clinic Procedure", "Creams", "Standard", 35, "", "", ""},
		{"Boost Lift", "Clinic Procedure", "Creams", "Standard", 35, "", "", ""},
		{"Boost Mat", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"Boost Relax", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"Hair Care Serum", "Clinic Procedure", "Creams", "Pharmaceries", 40, "", "", ""},
		{"Retinol Serum", "Clinic Procedure", "Creams", "Pharmaceries", 35, "", "", ""},
		{"Whitening Cream", "Clinic Procedure", "Creams", "Pharmaceries", 30, "", "", ""},
		// --- Clinic Procedure - Others ---
		{"Filler Dissolver", "Clinic Procedure", "Others", "General", 150, "", "", ""},
		{"Triple Enzymes", "Clinic Procedure", "Others", "General", 100, "", "", ""},
		{"PRP (Platelet-Rich Plasma)", "Clinic Procedure", "Others", "General", 200, "", "", ""},
		{"Consultation with Dr. Joe", "Clinic Procedure", "Others", "General", 50, "", "", ""},
		// --- Hospital Surgery ---
		{"Rhinoplasty", "Hospital Surgery", "Face", "Nose", 0, "Consultation required", "General anesthesia", ""},
		{"Blepharoplasty (Upper)", "Hospital Surgery", "Face", "Eye Surgery", 0, "Consultation required", "Local or general anesthesia", ""},
		{"Blepharoplasty (Lower)", "Hospital Surgery", "Face", "Eye Surgery", 0, "Consultation required", "Local or general anesthesia", ""},
		{"Facelift", "Hospital Surgery", "Face", "Facelift", 0, "Consultation required", "General anesthesia", ""},
		{"Neck Lift", "Hospital Surgery", "Face", "Facelift", 0, "Consultation required", "General anesthesia", ""},
		{"Otoplasty", "Hospital Surgery", "Face", "Ears", 0, "Consultation required", "Local or general anesthesia", ""},
		{"Liposuction", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", ""},
		{"Abdominoplasty", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", ""},
		{"Brazilian Butt Lift", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", ""},
		{"Breast Augmentation", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		{"Breast Reduction", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		{"Breast Lift", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		{"Gynecomastia", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		// --- Minor Surgery ---
		{"Mole Removal", "Minor Surgery", "Skin", "Lesions", 150, "", "Local anesthesia", ""},
		{"Cyst Removal", "Minor Surgery", "Skin", "Lesions", 200, "", "Local anesthesia", ""},
		{"Lipoma Removal", "Minor Surgery", "Skin", "Lesions", 250, "", "Local anesthesia", ""},
		{"Skin Tag Removal", "Minor Surgery", "Skin", "Lesions", 50, "per tag", "", ""},
		{"Wart Removal", "Minor Surgery", "Skin", "Lesions", 50, "per wart", "", ""},
		{"Scar Revision", "Minor Surgery", "Skin", "Skin Reconstruction", 300, "", "Local anesthesia", ""},
		{"Earlobe Repair", "Minor Surgery", "Face", "Face Reconstruction", 200, "", "Local anesthesia", ""},
		{"Fat Transfer (Face)", "Minor Surgery", "Face", "Face Enhancement", 0, "Consultation required", "Local anesthesia + sedation", ""},
		{"Thread Lift", "Minor Surgery", "Face", "Face Enhancement", 0, "Consultation required", "Local anesthesia", ""},
		{"PRP Hair Restoration", "Minor Surgery", "Hair", "Restoration", 250, "per session", "", ""},
	}

	for _, s := range services {
		typeID := typeMap[s.procType]
		categoryID := catMap[s.category+"/"+s.subcategory]

		var existingID string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM procedures WHERE name = ? AND type_id = ? AND category_id = ?`,
			s.name, typeID, categoryID,
		).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		procID := existingID
		if err == sql.ErrNoRows {
			procID = uuid.Must(uuid.NewV7()).String()
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO procedures (id, name, type_id, category_id, price_note, is_active, remarks, includes, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, 1, ?, ?, datetime('now'), datetime('now'))`,
				procID, s.name, typeID, categoryID, s.priceNote, s.remarks, s.includes,
			); err != nil {
				return err
			}
		}

		var priceExists string
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM procedure_prices WHERE procedure_id = ? AND is_active = 1`, procID,
		).Scan(&priceExists)
		if err == sql.ErrNoRows {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO procedure_prices (id, procedure_id, price, is_active, created_at)
				 VALUES (?, ?, ?, 1, datetime('now'))`,
				uuid.Must(uuid.NewV7()).String(), procID, s.price,
			); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}

func downSeedProcedures(ctx context.Context, tx *sql.Tx) error {
	return nil
}
