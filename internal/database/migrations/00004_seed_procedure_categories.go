package migrations

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedProcedureCategories, downSeedProcedureCategories)
}

func upSeedProcedureCategories(ctx context.Context, tx *sql.Tx) error {
	type catSeed struct {
		name     string
		children []string
	}

	categories := []catSeed{
		{"Botox", []string{"Women", "Men"}},
		{"Face", []string{
			"Face Fillers",
			"Face Skin Boosters",
			"Face Morpheus8",
			"Face CO2 Laser",
			"Nose",
			"Eye Surgery",
			"Facelift",
			"Ears",
			"Face Reconstruction",
			"Face Enhancement",
		}},
		{"Body", []string{
			"Body Fillers",
			"Body Skin Boosters",
			"Body Morpheus8",
			"Body CO2 Laser",
			"Body Contouring",
			"Breast",
		}},
		{"Eyes", []string{"Eye Skin Boosters"}},
		{"Laser Hair Removal", []string{"Single Areas", "Packages"}},
		{"Quanta Machine", []string{"Tattoo Removal", "Varicose", "Melasma", "Other"}},
		{"CO2 Laser", []string{"Sessions"}},
		{"Creams", []string{"Standard", "Pharmaceries"}},
		{"Others", []string{"General"}},
		{"Skin", []string{"Lesions", "Skin Reconstruction"}},
		{"Hair", []string{"Restoration"}},
	}

	for _, cat := range categories {
		parentID, err := upsertProcedureCategoryTx(ctx, tx, cat.name, "")
		if err != nil {
			return err
		}
		for _, child := range cat.children {
			if _, err := upsertProcedureCategoryTx(ctx, tx, child, parentID); err != nil {
				return err
			}
		}
	}
	return nil
}

func downSeedProcedureCategories(ctx context.Context, tx *sql.Tx) error {
	return nil
}

// upsertProcedureCategoryTx inserts the category if no row matches (name,
// parent_id), returning the resulting id. Existing rows are left untouched.
func upsertProcedureCategoryTx(ctx context.Context, tx *sql.Tx, name, parentID string) (string, error) {
	var existingID string
	err := tx.QueryRowContext(ctx, `SELECT id FROM procedure_categories WHERE name = ? AND parent_id = ?`, name, parentID).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id := uuid.Must(uuid.NewV7()).String()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO procedure_categories (id, name, parent_id, created_at) VALUES (?, ?, ?, datetime('now'))`,
		id, name, parentID,
	); err != nil {
		return "", err
	}
	return id, nil
}
