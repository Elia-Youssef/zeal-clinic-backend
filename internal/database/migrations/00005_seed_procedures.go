package migrations

import (
	"context"
	"database/sql"

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
		id, priceID, name, procType, category, subcategory string
		price                                              float64
		priceNote, remarks, includes                       string
	}

	services := []svcSeed{
		// --- Clinic Procedure - Botox (Women) ---
		{"b76c82c3-e031-45e3-b6c3-e51e7560a997", "5d101d46-1cd8-4ddc-a0d5-ef0814e68764", "Botox Full", "Clinic Procedure", "Botox", "Women", 220, "", "", ""},
		{"249f486f-f972-4e04-9ca4-70ead849701d", "28fac37c-1df5-4873-a475-7c6ad02b80eb", "Botox Full (Dysport)", "Clinic Procedure", "Botox", "Women", 250, "", "", ""},
		{"0f34d283-aad2-4972-9f88-9a2b7349444c", "a99be990-037d-4150-ba46-3c134a503ad6", "Botox Full + Bunny Lines", "Clinic Procedure", "Botox", "Women", 250, "", "", ""},
		{"7f129966-7256-4062-bea0-97e819ee8e87", "e18fc409-7fab-4bfc-9683-19efa0d42ffe", "Botox Marionette Lines (DAO)", "Clinic Procedure", "Botox", "Women", 30, "", "", ""},
		{"bf4f5190-f8f2-40bc-b62d-ba1a63b4b55e", "4b0b7ec7-7e92-4b70-b485-52994e2927fd", "Botox Around Eyes", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		{"3e2fb14d-f3de-4e3d-95f7-cbc5a9071dfe", "17d92e4a-83f4-4b9c-ab73-c76f3b856554", "Botox Frown Lines", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		{"7abfe4a1-8744-4f05-ac59-2c62d290a359", "7bd66c1d-50b8-437f-8d14-ebcc80e9be98", "Botox Gummy Smile", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		{"314524dd-668f-4abb-b160-bf9c77c913dd", "2614c04b-38c1-4de9-9dbc-bfb1ae6dbf63", "Botox Clenching Teeth", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"20f63b48-1993-4bc8-a2d3-3f2ced227024", "ad6a8599-03ec-4bac-b379-5afc0ce5fecb", "Botox Sweating", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"25ced3f0-c6e3-43b4-93ee-a8c0961ced92", "141d8871-df6c-47a9-8506-fb568e9c1ee7", "Botox Migraine", "Clinic Procedure", "Botox", "Women", 400, "", "", ""},
		{"8271ad6c-59b8-48b5-85ae-b0b015c1c222", "1ce6bf9c-b0d9-43e9-ba2c-4b04dec1f577", "Botox Neck", "Clinic Procedure", "Botox", "Women", 220, "", "", ""},
		{"07aa44eb-1f12-4220-b537-a0e03ffa51ae", "965c20cc-5a17-469e-8388-1610619eaaaa", "Botox Calves", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"69b62741-125c-4a5c-8026-ec1537096156", "46075b0f-8704-47c2-8b2d-9e02acf7d490", "Botox Jaw", "Clinic Procedure", "Botox", "Women", 150, "", "", ""},
		{"d167e63c-92c8-4171-af90-498bed188d81", "96948dd9-81d8-49e5-990c-8f6251444fb8", "Traptox", "Clinic Procedure", "Botox", "Women", 350, "", "", ""},
		{"ae2fcee4-d3a9-4374-9e4c-c6cd5979245c", "14f6469f-1ba1-4521-a752-48792dd0a646", "Lip Flip", "Clinic Procedure", "Botox", "Women", 120, "", "", ""},
		// --- Clinic Procedure - Botox (Men) ---
		{"2ae61a4a-e56d-4282-817f-6d09fb8aa653", "ef8e386b-6f5e-4c59-ab09-4d2441fdc61f", "Botox Full", "Clinic Procedure", "Botox", "Men", 250, "", "", ""},
		{"fe5bb87f-43db-4636-9392-ed2567ce0f8b", "02a802e0-807f-466b-929b-150c53cf2a54", "Botox Full (Dysport)", "Clinic Procedure", "Botox", "Men", 280, "", "", ""},
		{"4df85e17-de2c-41ac-843c-ed79bc2b9a90", "20ddbf24-23fe-4e65-bb9a-3de6dc3b9e1b", "Botox Full + Bunny Lines", "Clinic Procedure", "Botox", "Men", 280, "", "", ""},
		{"7ed6465e-a8d5-4370-a4db-fc4595d77d65", "7125e4e7-cbc6-48f6-9d33-a424251df7ce", "Botox Marionette Lines (DAO)", "Clinic Procedure", "Botox", "Men", 30, "", "", ""},
		{"811bb0b0-a2f1-4e35-a014-1db017f1e208", "76979ea8-a8f8-4618-81fc-780b1a294882", "Botox Around Eyes", "Clinic Procedure", "Botox", "Men", 150, "", "", ""},
		{"cee354aa-28d4-44df-bc5c-6455257db3e3", "924be447-075a-4504-ba56-1698cff939c7", "Botox Frown Lines", "Clinic Procedure", "Botox", "Men", 150, "", "", ""},
		{"4b0efdf9-d8ae-48f9-b391-4626e97b68cf", "0c2254f0-337d-49a9-8f5f-97dd67eb8df0", "Botox Gummy Smile", "Clinic Procedure", "Botox", "Men", 120, "", "", ""},
		{"7643399f-4a53-44f7-b4a1-c3af1445fcd1", "466f4b0d-32e9-411f-b897-f5c77dd6e36f", "Botox Teeth Clenching", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"d0bca7fa-35fc-4a90-84ed-f83231651d8b", "decf0b74-9255-4d24-aede-0f9b35933bc7", "Botox Sweating", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"5d2895cc-272b-4664-884a-e5c26ba65a87", "a470a4e6-1385-4247-b528-93ed86e2a776", "Botox Migraine", "Clinic Procedure", "Botox", "Men", 400, "", "", ""},
		{"82996d55-118f-46c7-99fe-f92b1647406b", "e6fc408f-42ea-42ae-90ba-48730c83512b", "Botox Neck", "Clinic Procedure", "Botox", "Men", 250, "", "", ""},
		{"e31bf3e0-1fbe-45cd-9213-805e1c3bcee3", "77b492ad-abd3-449b-8bed-33d59d4394a8", "Botox Calves", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"f4c747a3-799b-44f4-8c1b-3eb7bd331cf6", "f3ea8cc3-55da-4e1b-83b9-1e28cf46ef63", "Botox Jaw", "Clinic Procedure", "Botox", "Men", 180, "", "", ""},
		{"f8b54851-862e-4b53-aa77-0c8c1b8e53d5", "c084d78b-27b2-4ef2-9d6a-3e22d0217bae", "Traptox", "Clinic Procedure", "Botox", "Men", 350, "", "", ""},
		{"5d6ea6f7-6c0a-441b-a259-f01fa8bae9c0", "963d697c-728c-465e-922f-8633a63e9acf", "Lip Flip", "Clinic Procedure", "Botox", "Men", 120, "", "", ""},
		// --- Clinic Procedure - Fillers ---
		{"b5c1a9e7-8bf9-4cf9-9202-526aa468423d", "3e4465f0-74bf-4dc8-852c-21b6e1b1af30", "Lips", "Clinic Procedure", "Face", "Face Fillers", 250, "", "", ""},
		{"0aeb6203-cb1c-4bcc-8047-9e14c02f3dcd", "d0e913a7-782c-4d63-96cf-6a34e6e3b3fc", "Nasolabial Folds", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"881de33a-d35a-4aca-b86c-ac21fa65f8e3", "97aefae0-b41b-478a-a0f3-5ab7af7ad197", "Marionette Lines", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"2f5f7024-fdd4-4bbf-97cc-142719fb4a7f", "89e2c301-a2de-48e5-bba3-2fd0dd0bed03", "Cheeks", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"217a0d51-a8c4-4de3-8991-776d04d2f586", "944276a6-e520-4c4f-92c9-b004b8c83895", "Jawline", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"0c573857-604d-4ab0-b5bd-de0522165377", "c0c49e2d-1b1d-45e6-afe1-fe877909d9f6", "Chin", "Clinic Procedure", "Face", "Face Fillers", 300, "", "", ""},
		{"090e3115-3f53-48f7-96e1-c85a29caacc2", "9c90eff1-71f1-4014-97c5-22440c43f059", "Under Eyes", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"95a46428-1334-4c89-8914-8fc5afb9d09c", "c7becebf-f6a8-48f3-9149-80cb6ca933e9", "Nose (Non-Surgical)", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"e5086213-5bfb-4ed8-b4a4-889a24670311", "b1f85cbd-9a1a-4863-a0da-dbd4f93d8632", "Temples", "Clinic Procedure", "Face", "Face Fillers", 350, "", "", ""},
		{"838a6edf-fab9-4936-9907-6146a49ad5e1", "196e5804-61e1-4864-9adc-efcd1cedc748", "Earlobes", "Clinic Procedure", "Face", "Face Fillers", 200, "", "", ""},
		{"709f644f-a1d0-4d8b-9e6d-2ae091b7ee9c", "020b0598-66d0-466c-bc6e-aaaa08af4a4a", "Hands", "Clinic Procedure", "Body", "Body Fillers", 350, "", "", ""},
		// --- Clinic Procedure - Skin Boosters ---
		{"a42d61a5-d10d-4d8b-af21-cdb093a63830", "ad50134f-dbd4-4af4-a1a7-3d4f0098ebf8", "Profhilo Face", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"6ff03360-f0fc-41f5-b81a-c6c4f24b26b1", "cfd70d35-b907-4696-b344-706a0804958d", "Profhilo Neck", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"e8620984-dfe8-4a94-8a48-4b36539d1a4d", "e07ca718-d2a7-4f81-b8bd-10272f458449", "Skinvive", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"705fa186-c574-40ee-826d-9b5e78ca87ba", "96f7c09d-fc77-4878-ab72-0166afe30635", "Sculptra", "Clinic Procedure", "Face", "Face Skin Boosters", 350, "", "", ""},
		{"fbef79b5-3b08-4493-ab7c-341d06c885c0", "59326d9f-d49b-4fac-b36b-26509e71e487", "Exosome", "Clinic Procedure", "Face", "Face Skin Boosters", 350, "", "", ""},
		{"1750a684-8382-4d9e-b2be-b3b290bd30d5", "840933f9-806d-4cf4-a492-6e340037c107", "Collagen Booster", "Clinic Procedure", "Face", "Face Skin Boosters", 250, "", "", ""},
		{"8cd7ff6d-4571-4559-84eb-1911c73cba2d", "87ed14ad-4dfe-4257-90a4-2016a9a70b3a", "Jalupro Face", "Clinic Procedure", "Face", "Face Skin Boosters", 200, "", "", ""},
		{"22de8d12-f81d-4a76-819c-22076b1db8c0", "a60e624b-7ff1-45b7-a67b-4b15b1392e48", "Jalupro Neck", "Clinic Procedure", "Face", "Face Skin Boosters", 200, "", "", ""},
		{"06b121e5-bc7f-4f09-b288-d71d7db376ac", "a699a44b-f038-4303-85bf-c0c7bdeea8e6", "Profhilo Body", "Clinic Procedure", "Body", "Body Skin Boosters", 350, "", "", ""},
		{"cf5bdf7f-72ae-4992-84c6-d587e570a662", "84edad69-63d1-495e-8d8d-d4920ef30093", "Jalupro Eye", "Clinic Procedure", "Eyes", "Eye Skin Boosters", 200, "", "", ""},
		{"d02a0e9b-00d5-4365-827a-465ab281b2c0", "402aeca8-36ca-4baa-97a0-359ea2cd160f", "Chroma Phill Art Eye", "Clinic Procedure", "Eyes", "Eye Skin Boosters", 200, "", "", ""},
		// --- Clinic Procedure - Morpheus8 ---
		{"2e775a89-6495-4405-b087-abdf344ebbee", "e2e6d5b7-e8d7-4e63-873c-579720309bc8", "Face + Plasma (1 session)", "Clinic Procedure", "Face", "Face Morpheus8", 350, "per session", "", ""},
		{"b2105066-c941-4fd9-b46c-6bf3b73d3d10", "2281b9eb-d67f-437e-a40a-91e0ed58ea05", "Face + Plasma (3 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 900, "", "", ""},
		{"71f567ae-a4bf-4496-bbf2-f44d5cac7688", "27b620e2-3115-4b80-ac48-f99591c0d50b", "Face + Plasma (4 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 1100, "", "", ""},
		{"49469fcf-85d5-47c2-97e4-2b545bf21281", "d104c612-6dca-41a9-acf8-a74e7235a866", "Neck + Plasma (1 session)", "Clinic Procedure", "Face", "Face Morpheus8", 250, "per session", "", ""},
		{"9c6f1301-ed68-41e7-950c-85d99074b8de", "536c2f59-241e-4d70-9f98-38d48f96e282", "Neck + Plasma (3 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 650, "", "", ""},
		{"1425a34d-e6be-45e7-8fb5-4876e220b8ae", "4f760a86-8ced-4874-9639-bf3caf0eaf7f", "Face & Neck + Plasma (1 session)", "Clinic Procedure", "Face", "Face Morpheus8", 500, "per session", "", ""},
		{"a426df35-e0e2-4cac-82ed-887a9facf3a2", "1193f124-8d96-4728-8f9a-946d6233a814", "Face & Neck + Plasma (3 sessions)", "Clinic Procedure", "Face", "Face Morpheus8", 1300, "", "", ""},
		{"d01f9b39-4d6a-4fae-8567-b64489072df7", "5e0a50f5-f89e-4a41-8b85-59625aec4d9f", "Arms", "Clinic Procedure", "Body", "Body Morpheus8", 350, "per session", "", ""},
		{"49c6ace7-869b-4f04-b6d1-6b4c797ceee7", "3e2d22f8-0702-44fa-9ad1-8f3bbbccb120", "Abdominal", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", ""},
		{"75bc5961-88b8-4c18-b22e-b81b8da31f08", "3a72b41c-4a87-48e9-af1c-c12852ac6eee", "Love Handles", "Clinic Procedure", "Body", "Body Morpheus8", 350, "per session", "", ""},
		{"03f1fe5b-29bd-491e-8822-ce9f8a351a40", "deb3e6dc-8cc4-4d1c-b255-5410eb79c865", "Thighs", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", ""},
		{"527ba4de-5f12-4b64-8a1a-c9ffd28187d1", "f1b2126a-8161-4e24-a641-f839ad50d35d", "Buttocks", "Clinic Procedure", "Body", "Body Morpheus8", 500, "per session", "", ""},
		{"57945c5e-caa8-4b0c-b20d-5e7b2fe4a10d", "66daa2cd-2e8c-48b2-8cd1-b6678d728416", "Hands", "Clinic Procedure", "Body", "Body Morpheus8", 250, "per session", "", ""},
		{"31c35207-0257-4259-b6c4-595d849e15a1", "84494ebf-a365-46c1-89d3-d73a01f83235", "Knees", "Clinic Procedure", "Body", "Body Morpheus8", 250, "per session", "", ""},
		// --- Clinic Procedure - Laser Hair Removal ---
		{"69b865cd-8c5f-4f0d-9df1-eaa26afc152d", "9a43f205-ce5f-4778-b7f0-738da671ae84", "Full Face", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"4b195337-ce91-4ecf-864f-dfb807448be1", "feae9ae8-7634-44f3-8afb-01e9b65da9d9", "Upper Lip", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 15, "", "", ""},
		{"871ed25a-0140-4708-8ee9-98efd88c5d72", "ddc07ea3-21f3-42e6-8662-3604c1f0dfe5", "Sideburns", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 20, "", "", ""},
		{"5d5fff90-8c0c-43a5-850f-d41579568066", "10a7dfd2-3751-4d73-93dc-924e96afa70b", "Chin", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 15, "", "", ""},
		{"444993d2-6b0d-4b7c-9345-7a7e7e6e750d", "c7841f79-e5c6-4f2b-a84e-6dc0b4cfcc3a", "Full Arms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"92a1b4e4-9415-452e-8be0-2e8a0eb7ab24", "30a59e10-7ee1-4219-92f1-b0c83ade8c30", "Half Arms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"4e9bee7e-2fa8-48c8-8fb0-8d4d9883731c", "416c1b6f-2f03-4621-b11b-079199f0bdd9", "Underarms", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 25, "", "", ""},
		{"f2814899-09b5-4c5d-abee-7baba4b060fe", "b25ebf6d-d8cd-4354-9fc8-82896b11f322", "Full Legs", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 80, "", "", ""},
		{"34071677-c103-4631-b867-5f8e1ef8e1e2", "4ae8d10f-7842-4389-9bf5-b45b495e3bde", "Half Legs", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", ""},
		{"518659d6-621b-4d5b-8c8f-1be498788430", "7eb976bd-5ceb-4cd0-9fe2-683f599f11da", "Bikini", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"e537bffc-cc90-4822-b061-9b209488b772", "f5317607-6d44-43a9-b373-082807169bed", "Brazilian", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"f2a62c71-b59f-4ed1-b4d1-3eba3b02ca49", "ff7cca7b-a232-4d8b-8468-f22548a0d7e4", "Chest", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", ""},
		{"bd28532d-e7a0-41a9-9df1-abdaeecff6a5", "0e810ea9-e0e8-45bb-a6a9-234594aecb60", "Abdomen", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 50, "", "", ""},
		{"1dd91bb2-7e9a-4c2b-8806-852b4f742964", "3de5025c-6467-4b67-8a6f-a28d8bfdfdea", "Full Back", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 60, "", "", ""},
		{"4651e36b-905a-43a1-aaa2-89fd9ef623d9", "82d17ed3-a2de-43ca-b940-4fcfae5203ba", "Half Back", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"bdce352e-813a-4ee1-a022-476d3fbe541d", "578ae6d7-b18c-42cc-b4a1-2c9a29e46678", "Neck", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 25, "", "", ""},
		{"b1399619-de3a-4c95-98df-1b4fd6a470e2", "ebb64eb7-72f9-47dd-9dec-a6b1bae2afbd", "Buttocks", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 40, "", "", ""},
		{"2500098b-55ab-458b-b387-51b95972a08f", "38c19db0-a94d-454f-933e-e1608cf91417", "Hands / Feet", "Clinic Procedure", "Laser Hair Removal", "Single Areas", 20, "", "", ""},
		{"23a0ec49-541a-403e-9f2d-aaad069f61a6", "ba0e4093-8705-454a-a2ff-9753cbfab788", "Full Body Package 1", "Clinic Procedure", "Laser Hair Removal", "Packages", 200, "", "", "Full Face, Underarms, Full Arms, Brazilian"},
		{"c5a9d461-2adf-41d7-8221-84dc26652ad5", "494fb8cb-dea2-467a-a766-41658c028a96", "Full Body Package 2", "Clinic Procedure", "Laser Hair Removal", "Packages", 250, "", "", "Full Face, Underarms, Full Arms, Brazilian, Full Legs"},
		{"01e75d12-7dd7-4b00-aa7c-4a3a7e3c9b8b", "b23c45a2-4777-4dbe-8086-fd0ef80927e3", "Full Body Package 3", "Clinic Procedure", "Laser Hair Removal", "Packages", 300, "", "", "Full Face, Underarms, Full Arms, Brazilian, Full Legs, Full Back"},
		// --- Clinic Procedure - Quanta Machine ---
		{"de8faaed-262b-4363-b998-b49eab6b7e21", "e8102922-a689-4e06-bcfe-6a9eed435e79", "Tattoo Removal - Eyebrows", "Clinic Procedure", "Quanta Machine", "Tattoo Removal", 100, "", "", ""},
		{"93b15287-18ce-4575-9946-630819d451a3", "0b09c0ae-149e-4206-85f5-a4b5fdcd11a0", "Tattoo Removal - Face/Body", "Clinic Procedure", "Quanta Machine", "Tattoo Removal", 150, "", "", ""},
		{"eb61e5a2-b62f-4f39-a7d6-509845556c09", "02ed60f9-6def-40bf-a441-e6475d509473", "Varicose Vein - Per Vein", "Clinic Procedure", "Quanta Machine", "Varicose", 50, "per vein", "", ""},
		{"0941bd6a-5932-4552-b624-802fb5c3b1e6", "07e9d5e4-2c3a-403d-a2fe-6553f2c29911", "Varicose - Full Face", "Clinic Procedure", "Quanta Machine", "Varicose", 150, "", "", ""},
		{"9d015393-e93b-40c3-ade0-0053018e5d08", "10621809-3f14-4aee-aafb-3c0f24978e4e", "Varicose - Full Body", "Clinic Procedure", "Quanta Machine", "Varicose", 300, "", "", ""},
		{"97e43fde-3bf0-4b14-9f95-343f02418467", "fcc3f950-1807-461e-8e29-37acd4ebdc54", "Melasma Q-Switched", "Clinic Procedure", "Quanta Machine", "Melasma", 100, "", "", ""},
		{"f9484b1c-e4cc-4fa9-96d0-ed095fdfe36f", "179e0983-a19b-4105-b0b5-7f921d049166", "Melasma Full Face", "Clinic Procedure", "Quanta Machine", "Melasma", 200, "", "", ""},
		{"87175ddd-afa6-4581-8ff7-95cbaaa3007b", "c0d1e1d5-e4e1-4f58-a07b-d20b8ad5454a", "Carbon Peel", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		{"27608e44-e6d7-4633-be9a-67ca5154000d", "2ee50562-5f32-4c75-81c1-e18820d036d2", "Hair Bleaching", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		{"efc6b56e-0001-4c1d-a319-df05ab0d6370", "06f263ee-de6e-4bc6-ad8f-afd95a0a153f", "Rosacea Treatment", "Clinic Procedure", "Quanta Machine", "Other", 150, "", "", ""},
		{"ee90448d-b084-4387-aac0-feefb6780eb8", "d158674b-72ee-45df-b3af-aa2875f2b82d", "Scar Treatment", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		{"a49011bf-ec35-4ff8-ba49-6a1c785cb1e4", "b5e3ae61-38f3-46cd-9074-9543ae87c42d", "Cherry Angiomas", "Clinic Procedure", "Quanta Machine", "Other", 50, "", "", ""},
		{"b2deea67-6526-4dc8-97b1-d0bbd01874b5", "ca5ce157-5839-4800-aedd-beee96f8ccf7", "Freckles Removal", "Clinic Procedure", "Quanta Machine", "Other", 100, "", "", ""},
		// --- Clinic Procedure - CO2 Laser ---
		{"09d48f17-2100-4f45-82d9-ef029f0051e6", "a4963f3e-a1fc-42d8-8a6a-917f7667e403", "Single Session", "Clinic Procedure", "CO2 Laser", "Sessions", 200, "", "", ""},
		{"36ea9e31-1129-4b60-a77b-a84d93eb8f5e", "0f24c85f-aaa2-4a02-a94f-270d22ff6a4a", "3-Session Package", "Clinic Procedure", "CO2 Laser", "Sessions", 500, "", "", ""},
		{"48cd58e2-7be2-4226-a71f-b996a6485fa6", "bde6503e-0289-4a4b-bd9e-0b4a6c97dec8", "Under Eyes", "Clinic Procedure", "Face", "Face CO2 Laser", 150, "", "", ""},
		{"d885f24a-9c7e-4abb-90e9-b8842fa81c46", "57517162-c98e-44c8-ba40-7c064808be4b", "Full Face", "Clinic Procedure", "Face", "Face CO2 Laser", 300, "", "", ""},
		{"1db662f6-4b45-4d88-a2d2-f162c7be1a0d", "c390eb41-4e73-48b1-9750-8faac47893b2", "Neck", "Clinic Procedure", "Face", "Face CO2 Laser", 200, "", "", ""},
		{"f793b009-c4b7-4800-b3b0-a2eb8042c541", "9d90053d-a3a8-4ab4-b588-73972e065d7c", "Body Area", "Clinic Procedure", "Body", "Body CO2 Laser", 250, "", "", ""},
		// --- Clinic Procedure - Creams ---
		{"a2d4a19f-3316-4a50-8b1a-7f3b73a83c70", "86569a38-095b-4988-9c0d-baf2ba129937", "Sodermix", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"8befbdcf-98cb-47c3-9119-d27f03e50797", "1bd874ef-907f-4c1d-8b05-f26791f636b4", "Keloplast", "Clinic Procedure", "Creams", "Standard", 25, "", "", ""},
		{"24f7786a-527b-43bc-9b46-6e330915bbb7", "63f44e44-3615-40f8-9244-13b963a03afc", "Beclean", "Clinic Procedure", "Creams", "Standard", 20, "", "", ""},
		{"3c3721b4-844d-4296-8da7-30ec967d91e9", "53d6334d-9c72-40f3-ac07-226e0c9fb29f", "Boost C", "Clinic Procedure", "Creams", "Standard", 35, "", "", ""},
		{"e905b9f0-5e26-4361-89d0-1fd47c35cd1e", "6302522e-2270-4572-aed5-5bd351c2c6e4", "Boost Eye", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"07bef9e6-2ff3-4212-87e0-55778824e4ea", "6d7268b5-f877-4145-855a-818f969a9633", "Boost Glow", "Clinic Procedure", "Creams", "Standard", 35, "", "", ""},
		{"6c021d01-f565-4d17-84e3-d7fc31d2740a", "723ebfec-4079-47c2-a8d7-a3aefe7c5d67", "Boost Lift", "Clinic Procedure", "Creams", "Standard", 35, "", "", ""},
		{"34864d53-f6ad-469f-b8da-f97c0e95fa81", "ffb7c19c-9b0c-49f0-83bf-7f468e14a73e", "Boost Mat", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"56a2651d-599c-4449-b776-2eb669eabd98", "25271391-6622-4bef-8c52-09e17ed20e66", "Boost Relax", "Clinic Procedure", "Creams", "Standard", 30, "", "", ""},
		{"0d6f49c9-d2db-49a3-8f00-4a6dfc654a05", "53200802-cfca-4e09-8825-034970c4e932", "Hair Care Serum", "Clinic Procedure", "Creams", "Pharmaceries", 40, "", "", ""},
		{"d7dd6b0a-e67b-4d39-a2a2-a187b34038f7", "4b9194b4-114a-4aa9-8d5a-eebd356ba509", "Retinol Serum", "Clinic Procedure", "Creams", "Pharmaceries", 35, "", "", ""},
		{"95376dea-b758-4f1b-be37-852301dc4f80", "a9d4a55e-00ef-41f7-a336-65f562945cf8", "Whitening Cream", "Clinic Procedure", "Creams", "Pharmaceries", 30, "", "", ""},
		// --- Clinic Procedure - Others ---
		{"6ef97e2f-eab3-4cbf-bc76-8629ac788f16", "3d20f586-08ae-4304-af47-e1f8c68704bc", "Filler Dissolver", "Clinic Procedure", "Others", "General", 150, "", "", ""},
		{"757244f6-f6ba-4eee-a6c5-e9c75a301ea1", "9746f9e1-c971-4c8d-9bfa-fd79c6988fa3", "Triple Enzymes", "Clinic Procedure", "Others", "General", 100, "", "", ""},
		{"166ec1f6-e7f0-4703-a86d-152fc7d87278", "ddfde2bf-9f50-4537-b672-2f4d6cd57172", "PRP (Platelet-Rich Plasma)", "Clinic Procedure", "Others", "General", 200, "", "", ""},
		{"6df0a12d-18ae-48a7-8d26-fb8fa70f4a31", "f93b554a-166f-4a86-82d5-ad4fc699269d", "Consultation with Dr. Joe", "Clinic Procedure", "Others", "General", 50, "", "", ""},
		// --- Hospital Surgery ---
		{"cd8509ac-b384-4402-8999-02bccea7590b", "954261c1-69c2-4a75-b0a1-6c24264683b5", "Rhinoplasty", "Hospital Surgery", "Face", "Nose", 0, "Consultation required", "General anesthesia", ""},
		{"e031dd76-d775-4273-982a-60a97b232395", "f6d7026b-363a-4721-beb8-cdd1f8134a62", "Blepharoplasty (Upper)", "Hospital Surgery", "Face", "Eye Surgery", 0, "Consultation required", "Local or general anesthesia", ""},
		{"e677a443-f134-417a-b2a3-dc780a4335cb", "d2374887-9682-41bb-988d-7259d9c3d829", "Blepharoplasty (Lower)", "Hospital Surgery", "Face", "Eye Surgery", 0, "Consultation required", "Local or general anesthesia", ""},
		{"253b53d0-014a-4b74-9cf3-09e99f54039b", "a8633ecd-8afc-4608-82d6-c574038eccc0", "Facelift", "Hospital Surgery", "Face", "Facelift", 0, "Consultation required", "General anesthesia", ""},
		{"88a6399d-0bda-4c30-995e-c13414af83f1", "68402217-0f2a-4a77-9230-33f15df1bebe", "Neck Lift", "Hospital Surgery", "Face", "Facelift", 0, "Consultation required", "General anesthesia", ""},
		{"bef9f587-127d-4d48-b775-4b53dec60275", "36948cae-6f3d-4501-b16f-5170523305f8", "Otoplasty", "Hospital Surgery", "Face", "Ears", 0, "Consultation required", "Local or general anesthesia", ""},
		{"f5c255ca-9cf7-4ced-8557-1b6df564ae8f", "26f7d22f-1531-4124-bfbf-2cfd3d0bac28", "Liposuction", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", ""},
		{"6686816c-297f-4661-8dfb-555e54f5c8b4", "a6198cbf-6eb9-431b-8996-ae92023d0687", "Abdominoplasty", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", ""},
		{"6f7dea5c-4c31-4a8c-b199-473df512e9f5", "c62599f0-9b63-4126-96d2-7a3e57000abe", "Brazilian Butt Lift", "Hospital Surgery", "Body", "Body Contouring", 0, "Consultation required", "General anesthesia", ""},
		{"bf22f839-c3ca-4865-8340-881ad3da626c", "1fee2e42-0c4d-4ab4-866e-91d4f4949d95", "Breast Augmentation", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		{"0829d97d-7b9a-4cc4-9331-1cbe05315e7b", "9ce13b82-9382-47ac-8a70-b1b7f9e11f8d", "Breast Reduction", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		{"d416d7b6-cd5a-4fe5-a54d-bc39bea81f40", "3fda2f28-a143-45d0-b07b-a9921d77202d", "Breast Lift", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		{"ab519801-6d1e-4b14-ad85-18aba640bcad", "a995bba4-29f0-4fe3-ae1a-256ee3c43fbb", "Gynecomastia", "Hospital Surgery", "Body", "Breast", 0, "Consultation required", "General anesthesia", ""},
		// --- Minor Surgery ---
		{"582385aa-f72b-4d65-bed4-60f58ec56b96", "3b6d5146-350e-4404-bbdb-eb1485c6cffd", "Mole Removal", "Minor Surgery", "Skin", "Lesions", 150, "", "Local anesthesia", ""},
		{"908b8ab3-6e97-4741-8415-ff1e6da69dc9", "e0184cce-4f63-42a4-acb0-da6e90a9b0fc", "Cyst Removal", "Minor Surgery", "Skin", "Lesions", 200, "", "Local anesthesia", ""},
		{"9b0e651a-4425-404f-a360-1f228bbc7abb", "f3688b14-d23f-400a-80f6-7bf1d0184b23", "Lipoma Removal", "Minor Surgery", "Skin", "Lesions", 250, "", "Local anesthesia", ""},
		{"22da2dbc-89e1-48ac-903c-4625edeca0ac", "8ee00ee9-324c-4432-b9a3-6599181c4e31", "Skin Tag Removal", "Minor Surgery", "Skin", "Lesions", 50, "per tag", "", ""},
		{"de374b43-dcfc-4f25-985e-78916f8a9d0e", "aa1e689d-fd25-46c2-a6bf-899ea05fed5c", "Wart Removal", "Minor Surgery", "Skin", "Lesions", 50, "per wart", "", ""},
		{"9a69819b-5d98-49cb-8781-86defce51265", "705a4adf-f68f-44a7-a0f3-6897148e77a5", "Scar Revision", "Minor Surgery", "Skin", "Skin Reconstruction", 300, "", "Local anesthesia", ""},
		{"1476308e-0c2d-48e7-a090-00fdcd406298", "5232353d-2a32-46f1-8eb5-e2f8fd65ba7d", "Earlobe Repair", "Minor Surgery", "Face", "Face Reconstruction", 200, "", "Local anesthesia", ""},
		{"c2f4814a-f90d-49c5-bf6b-93916e2b3896", "43fbe613-40ef-4285-aeeb-8765b5738905", "Fat Transfer (Face)", "Minor Surgery", "Face", "Face Enhancement", 0, "Consultation required", "Local anesthesia + sedation", ""},
		{"1fda5a64-f8e6-45bd-b0e3-0af912f4fc60", "e4c81e9e-a6f7-49a0-b1ae-247a5d620b2d", "Thread Lift", "Minor Surgery", "Face", "Face Enhancement", 0, "Consultation required", "Local anesthesia", ""},
		{"8e58f096-2f77-4788-83cf-2b7db4871656", "52910958-5261-4cff-971d-abaa7bb306f3", "PRP Hair Restoration", "Minor Surgery", "Hair", "Restoration", 250, "per session", "", ""},
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
			procID = s.id
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO procedures (id, name, type_id, category_id, price_note, is_active, remarks, includes, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, 1, ?, ?, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
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
				 VALUES (?, ?, ?, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))`,
				s.priceID, procID, s.price,
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
