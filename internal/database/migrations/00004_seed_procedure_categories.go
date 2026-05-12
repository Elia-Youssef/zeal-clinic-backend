package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upSeedProcedureCategories, downSeedProcedureCategories)
}

func upSeedProcedureCategories(ctx context.Context, tx *sql.Tx) error {
	type child struct{ id, name string }
	type catSeed struct {
		id, name string
		children []child
	}

	categories := []catSeed{
		{"0c97a5da-b5a9-422c-afad-9b746f1758c3", "Botox", []child{
			{"3e131b15-f735-47cb-a03f-bb12a6ef0f27", "Women"},
			{"2cd72216-9d45-48f0-990b-e65d4b91be57", "Men"},
		}},
		{"57992e0d-e594-4a42-9e60-a0193d5f62d9", "Face", []child{
			{"a881f457-39a7-461b-87de-19e327966588", "Face Fillers"},
			{"46c6caf3-f84e-4059-9a88-650a880c85af", "Face Skin Boosters"},
			{"c18091d0-68ae-4691-93ac-5bd81dd79dbe", "Face Morpheus8"},
			{"a7a4dcc0-faab-4ed9-b931-14e738f1a7d9", "Face CO2 Laser"},
			{"981b97e0-2ad8-4358-a50c-9d69455f258e", "Nose"},
			{"425a2700-d277-49b3-be08-9242e29729da", "Eye Surgery"},
			{"c2a49739-2e56-4a17-9acf-cc68bfeedfeb", "Facelift"},
			{"bbd6e66c-aeff-42da-b9cf-ea250b7d64d5", "Ears"},
			{"bddcf892-28ee-49b6-8525-1d18df7cd3ed", "Face Reconstruction"},
			{"3834b32f-95c4-4ecb-89d0-76c5b4b03e80", "Face Enhancement"},
		}},
		{"8ccda229-75e0-4152-8ad5-51f5cc19ab28", "Body", []child{
			{"d8470044-1274-4db9-83a7-c4301a1e982b", "Body Fillers"},
			{"d39bde74-46c1-48e0-8250-7f6db9744c44", "Body Skin Boosters"},
			{"eeda26b5-1aef-4a10-acdf-0908e74aef78", "Body Morpheus8"},
			{"9bbefdab-d047-4a16-96d1-046105a01769", "Body CO2 Laser"},
			{"c9b7c4d3-a0aa-4712-8b6d-ec7749d991f8", "Body Contouring"},
			{"49b6b690-28bc-4ab0-803a-05a78e751980", "Breast"},
		}},
		{"464769bf-8632-4eeb-9a17-248fa5d9e171", "Eyes", []child{
			{"eeffc164-2be2-40d5-8848-b0dbbb381c86", "Eye Skin Boosters"},
		}},
		{"151a9c65-6181-439d-88e6-1a4dbc85d167", "Laser Hair Removal", []child{
			{"7800bd20-acd8-4aab-ac5a-558f752d688d", "Single Areas"},
			{"e81a63db-ad11-4b3d-8a31-52ea1babd290", "Packages"},
		}},
		{"66fef687-1c31-4cb7-a4d6-fea3100f7f09", "Quanta Machine", []child{
			{"14c6eae1-22a5-4efb-b6b1-df8bb38271fb", "Tattoo Removal"},
			{"27882995-f00c-4204-9d53-45eaee86f15c", "Varicose"},
			{"9c22212a-deee-488d-9a62-d04c776a3e4a", "Melasma"},
			{"27522cc8-d860-4c1f-a173-d0cb0481e696", "Other"},
		}},
		{"0162108c-9d98-4c85-92e0-4fedc338612b", "CO2 Laser", []child{
			{"f05e392d-8b4a-4af2-8b3b-5275b38f0458", "Sessions"},
		}},
		{"375ca07f-2e1a-4901-a59b-578df7b7e2dc", "Creams", []child{
			{"640ae85f-285b-4ddb-b5ed-0dd967b343ac", "Standard"},
			{"d6b08394-b459-46d7-8b06-0761e820d810", "Pharmaceries"},
		}},
		{"1047f7bf-e002-4f82-9041-3111e42ac69c", "Others", []child{
			{"e9b70711-35c0-4f24-a338-08f354d53385", "General"},
		}},
		{"e9742eac-de22-47b3-846a-a49f3638ce30", "Skin", []child{
			{"6f433c05-dd98-4fce-938e-e10e49effe56", "Lesions"},
			{"397ae3fe-34d0-45d1-80c7-73429b73fc35", "Skin Reconstruction"},
		}},
		{"84393b9c-ebd3-4f82-9be5-16b929727635", "Hair", []child{
			{"dcd7dda8-14e0-4598-9253-321d1c57cc88", "Restoration"},
		}},
	}

	for _, cat := range categories {
		if err := upsertProcedureCategoryTx(ctx, tx, cat.id, cat.name, ""); err != nil {
			return err
		}
		for _, ch := range cat.children {
			if err := upsertProcedureCategoryTx(ctx, tx, ch.id, ch.name, cat.id); err != nil {
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
// parent_id). Existing rows are left untouched; their id is not reconciled
// with the static seed id (this matters only for installs that pre-date
// static seeding).
func upsertProcedureCategoryTx(ctx context.Context, tx *sql.Tx, id, name, parentID string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO procedure_categories (id, name, parent_id, created_at)
		 SELECT ?, ?, ?, datetime('now')
		 WHERE NOT EXISTS (SELECT 1 FROM procedure_categories WHERE name = ? AND parent_id = ?)`,
		id, name, parentID, name, parentID,
	)
	return err
}
