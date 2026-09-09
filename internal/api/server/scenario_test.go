package server

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"clinic-api/internal/database/store"
)

// The contract scenario: a fixed dataset with obviously fake names, built on a
// fresh database through the API only. Business dates are fixed wherever the
// API takes them; the DST weeks are those of 2025 in the clinic's zone. A few
// appointments sit a fixed number of days from the run's clinic-local day, at
// fixed UTC times, for the "today" and "upcoming" views.

// Seeded rows the scenario uses.
const (
	seedRoom1      = "99ca4a8e-9d61-415a-9afb-4f563b11242d"
	seedRoom2      = "25e6a41c-c76b-4b39-b959-07cf51f4305e"
	seedRoom3      = "27fa19ea-2e15-406b-b8af-a1f83ee9dd67"
	seedRoom4      = "94214e6b-30c5-4173-b766-9765188d2102"
	seedClinicType = "4347efc8-5f3d-46fc-8e1c-0620d8b6e4ec"
	seedLebanon    = "931e6eda-fa3f-4e7c-bb3a-0ea9c6a51645"
	seedBeirut     = "6809a457-195a-4735-9283-e91e21be7883"
	seedJounieh    = "5390fb25-9407-45fb-805b-71eff5d4fa16"
	seedSuperAdmin = "b10829b3-19a0-4813-8e1c-df6f5990737b"
)

type contractScenario struct {
	admin, nurse, staff, unlinked, retired *contractActor

	adminUser, unlinkedUser, noPassUser, retiredUser string
	noraUser, samUser                                string

	latex, penicillin, pollen string
	amoxi, ibu                string
	room                      string
	aesthetic                 string
	skinCare, peels           string
	peel, laser               string
	peelLatex                 string
	skincare, serums          string
	serum, cream              string
	creamPollen               string
	acme, globex              string
	rent, utilities           string
	spring, fixedOffer        string

	ada, ben, cleo       string
	adaLatex, adaAmoxi   string
	nora, sam, otto      string
	adaRx                string
	noraShift, samShift  string
	timeoff, overtime    string
	noraSalary           string
	noraPrep             string
	holiday              string
	apptDST1, apptDST2   string
	apptMoved, apptLate  string
	apptCancelled        string
	apptHoliday, apptSun string
	apptPast, apptSoon   string

	invoice1, invoice2 string
	giftCode, giftID   string
	supplierInvoice    string
	serumItem          string
	adaPayment         string
	acmePayment        string
	rentPayment        string
	noraPayment        string
	notifRead          string
	nurseNotif         string

	staffScopes []string
}

// clinicUTC turns a clinic-local wall-clock time into the UTC form the
// dashboard sends.
func clinicUTC(t *testing.T, date, clock string) string {
	t.Helper()
	ts, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, store.ClinicLocation())
	if err != nil {
		t.Fatal(err)
	}
	return ts.UTC().Format("2006-01-02T15:04:05.000Z")
}

// day is the run's clinic-local day shifted by offset days.
func (r *contractRun) day(offset int) string {
	return r.norm.today.AddDate(0, 0, offset).Format(store.DateFormat)
}

// checkDSTVectors pins the clinic zone's offsets on both sides of the 2025
// DST changes the scenario uses.
func checkDSTVectors(t *testing.T) {
	t.Helper()
	for _, v := range []struct{ date, clock, utc string }{
		{"2025-03-29", "10:00", "2025-03-29T08:00:00.000Z"},
		{"2025-03-31", "10:00", "2025-03-31T07:00:00.000Z"},
		{"2025-03-31", "00:30", "2025-03-30T21:30:00.000Z"},
		{"2025-10-25", "10:00", "2025-10-25T07:00:00.000Z"},
		{"2025-10-26", "09:00", "2025-10-26T07:00:00.000Z"},
		{"2025-10-27", "10:00", "2025-10-27T08:00:00.000Z"},
	} {
		if got := clinicUTC(t, v.date, v.clock); got != v.utc {
			t.Fatalf("clinic time %s %s is %s in UTC, want %s: the zone data changed", v.date, v.clock, got, v.utc)
		}
	}
}

func (r *contractRun) post(name, path string, body any, want int) *contractReply {
	return r.run(contractCall{name: name, method: http.MethodPost, path: path, body: body, want: want})
}

func (r *contractRun) put(name, path string, body any, want int) *contractReply {
	return r.run(contractCall{name: name, method: http.MethodPut, path: path, body: body, want: want})
}

func (r *contractRun) del(name, path string, want int) *contractReply {
	return r.run(contractCall{name: name, method: http.MethodDelete, path: path, want: want})
}

func (r *contractRun) create(name, path string, body any) string {
	return r.post(name, path, body, http.StatusCreated).id(r.t)
}

type jsonObject = map[string]any

func buildContractScenario(r *contractRun, s *contractScenario) {
	r.accounts(s)
	r.catalog(s)
	r.people(s)
	r.hr(s)
	r.appointments(s)
	r.finance(s)
	r.notifications(s)
}

func (r *contractRun) accounts(s *contractScenario) {
	t := r.t
	r.super = *r.login("super-admin", "super-admin", "super-admin-pw")
	r.super.name = ""
	s.adminUser = r.create("create admin user", "/api/users",
		jsonObject{"username": "admin.example", "displayName": "Admin Example", "role": "admin", "password": "admin-example-pw"})
	s.unlinkedUser = r.create("create staff user without an employee", "/api/users",
		jsonObject{"username": "unlinked.staff", "displayName": "Unlinked Staff", "role": "staff", "password": "unlinked-pw"})
	s.noPassUser = r.create("create user without a password", "/api/users",
		jsonObject{"username": "no.password", "displayName": "No Password", "role": "nurse"})
	s.retiredUser = r.create("create user to deactivate", "/api/users",
		jsonObject{"username": "retired.example", "displayName": "Retired Example", "role": "staff", "password": "retired-pw"})
	s.admin = r.login("admin", "admin.example", "admin-example-pw")
	s.unlinked = r.login("unlinked staff", "unlinked.staff", "unlinked-pw")
	s.retired = r.login("user to deactivate", "retired.example", "retired-pw")
	var staff struct {
		Scopes []string `json:"scopes"`
	}
	r.send(contractCall{method: http.MethodGet, path: "/api/roles/staff"}).data(t, &staff)
	s.staffScopes = staff.Scopes
}

func (r *contractRun) catalog(s *contractScenario) {
	// Allergies and medicines.
	s.latex = r.create("create allergy Latex", "/api/allergies", jsonObject{"name": "Test Latex", "description": "Natural rubber"})
	s.penicillin = r.create("create allergy Penicillin", "/api/allergies", jsonObject{"name": "Test Penicillin", "description": ""})
	s.pollen = r.create("create allergy Pollen", "/api/allergies", jsonObject{"name": "Test Pollen", "description": "Seasonal"})
	dust := r.create("create allergy to delete", "/api/allergies", jsonObject{"name": "Test Dust"})
	r.put("update allergy Latex", "/api/allergies/"+s.latex, jsonObject{"description": "Natural rubber, gloves"}, http.StatusOK)
	r.del("delete unused allergy", "/api/allergies/"+dust, http.StatusOK)

	s.amoxi = r.create("create medicine Amoxitest", "/api/medicines", jsonObject{"name": "Amoxitest 500", "description": "Capsule"})
	s.ibu = r.create("create medicine Ibutest", "/api/medicines", jsonObject{"name": "Ibutest 200", "description": "Tablet"})
	old := r.create("create medicine to delete", "/api/medicines", jsonObject{"name": "Oldtest 10"})
	r.put("update medicine Amoxitest", "/api/medicines/"+s.amoxi, jsonObject{"description": "Capsule, 500 mg"}, http.StatusOK)
	r.del("delete unused medicine", "/api/medicines/"+old, http.StatusOK)

	// Rooms and currencies.
	s.room = r.create("create room", "/api/rooms", jsonObject{"name": "Test Room A", "type": "Procedure", "isAvailable": true})
	r.put("update room", "/api/rooms/"+s.room, jsonObject{"type": "Consultation"}, http.StatusOK)
	tempRoom := r.create("create room to delete", "/api/rooms", jsonObject{"name": "Test Room Temp", "type": "General", "isAvailable": true})
	r.del("delete unused room", "/api/rooms/"+tempRoom, http.StatusOK)

	eur := r.create("create currency", "/api/currencies", jsonObject{"code": "EUR", "name": "Euro", "symbol": "EUR", "exchangeRate": 0.92})
	r.put("update currency", "/api/currencies/"+eur, jsonObject{"exchangeRate": 0.91}, http.StatusOK)
	r.del("delete unused currency", "/api/currencies/"+eur, http.StatusOK)

	// Procedure catalog.
	s.aesthetic = r.create("create procedure type", "/api/procedure-types", jsonObject{"name": "Test Aesthetic", "description": "Scenario type"})
	r.put("update procedure type", "/api/procedure-types/"+s.aesthetic, jsonObject{"description": "Scenario type, updated"}, http.StatusOK)
	tempType := r.create("create procedure type to delete", "/api/procedure-types", jsonObject{"name": "Test Temp Type"})
	r.del("delete unused procedure type", "/api/procedure-types/"+tempType, http.StatusOK)

	s.skinCare = r.create("create procedure category", "/api/procedure-categories", jsonObject{"name": "Test Skin Care", "description": "Parent"})
	s.peels = r.create("create procedure subcategory", "/api/procedure-categories",
		jsonObject{"name": "Test Peels", "description": "Child", "parentId": s.skinCare})
	r.put("update procedure subcategory", "/api/procedure-categories/"+s.peels, jsonObject{"description": "Chemical peels"}, http.StatusOK)
	tempCat := r.create("create procedure category to delete", "/api/procedure-categories", jsonObject{"name": "Test Temp Category"})
	r.del("delete unused procedure category", "/api/procedure-categories/"+tempCat, http.StatusOK)

	s.peel = r.create("create procedure Glow Peel", "/api/procedures", jsonObject{"name": "Test Glow Peel", "typeId": s.aesthetic,
		"categoryId": s.peels, "price": 150, "priceNote": "per session", "isActive": true, "remarks": "Scenario", "includes": "Mask"})
	s.laser = r.create("create procedure Laser Session", "/api/procedures", jsonObject{"name": "Test Laser Session", "typeId": seedClinicType,
		"categoryId": s.peels, "price": 300, "isActive": true})
	r.put("change the Glow Peel price", "/api/procedures/"+s.peel, jsonObject{"price": 160, "remarks": "Scenario, repriced"}, http.StatusOK)
	tempProc := r.create("create procedure to delete", "/api/procedures", jsonObject{"name": "Test Temp Procedure", "typeId": s.aesthetic,
		"categoryId": s.peels, "price": 10, "isActive": true})
	r.del("delete unused procedure", "/api/procedures/"+tempProc, http.StatusOK)

	s.peelLatex = r.create("add procedure allergy conflict", "/api/procedures/"+s.peel+"/allergy-conflicts",
		jsonObject{"allergyId": s.latex, "notes": "Latex gloves"})
	r.put("update procedure allergy conflict", "/api/procedure-allergy-conflicts/"+s.peelLatex, jsonObject{"notes": "Use nitrile gloves"}, http.StatusOK)
	laserPollen := r.create("add procedure allergy conflict to remove", "/api/procedures/"+s.laser+"/allergy-conflicts",
		jsonObject{"allergyId": s.pollen, "notes": ""})
	r.del("remove procedure allergy conflict", "/api/procedure-allergy-conflicts/"+laserPollen, http.StatusOK)

	// Product catalog.
	s.skincare = r.create("create product category", "/api/product-categories", jsonObject{"name": "Test Skincare", "description": "Parent"})
	s.serums = r.create("create product subcategory", "/api/product-categories",
		jsonObject{"name": "Test Serums", "description": "Child", "parentId": s.skincare})
	r.put("update product subcategory", "/api/product-categories/"+s.serums, jsonObject{"description": "Face serums"}, http.StatusOK)
	tempPCat := r.create("create product category to delete", "/api/product-categories", jsonObject{"name": "Test Temp Products"})
	r.del("delete unused product category", "/api/product-categories/"+tempPCat, http.StatusOK)

	s.serum = r.create("create product Serum", "/api/products",
		jsonObject{"name": "Test Serum", "categoryId": s.serums, "quantity": 0, "minThreshold": 3, "unitPrice": 40})
	s.cream = r.create("create product Cream", "/api/products",
		jsonObject{"name": "Test Cream", "categoryId": s.skincare, "quantity": 20, "minThreshold": 5, "unitPrice": 25})
	r.put("reprice product Cream", "/api/products/"+s.cream, jsonObject{"unitPrice": 27.5, "minThreshold": 4}, http.StatusOK)
	tempProduct := r.create("create product to delete", "/api/products",
		jsonObject{"name": "Test Temp Product", "categoryId": s.skincare, "quantity": 1, "unitPrice": 5})
	r.del("delete unused product", "/api/products/"+tempProduct, http.StatusOK)

	s.creamPollen = r.create("add product allergy conflict", "/api/products/"+s.cream+"/allergy-conflicts",
		jsonObject{"allergyId": s.pollen, "notes": "Contains flower extract"})
	r.put("update product allergy conflict", "/api/product-allergy-conflicts/"+s.creamPollen, jsonObject{"notes": "Contains pollen extract"}, http.StatusOK)
	serumPen := r.create("add product allergy conflict to remove", "/api/products/"+s.serum+"/allergy-conflicts",
		jsonObject{"allergyId": s.penicillin})
	r.del("remove product allergy conflict", "/api/product-allergy-conflicts/"+serumPen, http.StatusOK)

	// Suppliers, expenses, offers, holidays.
	s.acme = r.create("create supplier Acme", "/api/suppliers", jsonObject{"name": "Acme Test Supplies", "contact": "+961 1 000 111",
		"email": "orders@acme.example", "address": "1 Test Street", "notes": "Main supplier"})
	s.globex = r.create("create supplier Globex", "/api/suppliers", jsonObject{"name": "Globex Sample Traders", "contact": "01000222"})
	r.put("update supplier Globex", "/api/suppliers/"+s.globex, jsonObject{"notes": "Second supplier"}, http.StatusOK)
	tempSupplier := r.create("create supplier to delete", "/api/suppliers", jsonObject{"name": "Initech Temp Vendor"})
	r.del("delete unused supplier", "/api/suppliers/"+tempSupplier, http.StatusOK)

	s.rent = r.create("create expense Rent", "/api/expenses", jsonObject{"name": "Test Rent", "notes": "Monthly"})
	s.utilities = r.create("create expense Utilities", "/api/expenses", jsonObject{"name": "Test Utilities"})
	r.put("update expense Utilities", "/api/expenses/"+s.utilities, jsonObject{"notes": "Power and water"}, http.StatusOK)
	tempExpense := r.create("create expense to delete", "/api/expenses", jsonObject{"name": "Test Temp Expense"})
	r.del("delete unused expense", "/api/expenses/"+tempExpense, http.StatusOK)

	s.spring = r.create("create percentage offer", "/api/discounts",
		jsonObject{"name": "Test Spring Offer", "discountType": "offer", "valueType": "percentage", "value": 10})
	s.fixedOffer = r.create("create fixed offer", "/api/discounts",
		jsonObject{"name": "Test Fixed Offer", "discountType": "offer", "valueType": "fixed", "value": 20, "description": "Flat"})
	r.put("update percentage offer", "/api/discounts/"+s.spring, jsonObject{"description": "Ten percent off"}, http.StatusOK)
	tempOffer := r.create("create offer to delete", "/api/discounts",
		jsonObject{"name": "Test Temp Offer", "discountType": "offer", "valueType": "fixed", "value": 1})
	r.del("delete unused offer", "/api/discounts/"+tempOffer, http.StatusOK)

	s.holiday = r.create("create holiday", "/api/holidays",
		jsonObject{"name": "Test Holiday", "startDate": "2025-10-27", "endDate": "2025-10-27", "notes": "Scenario"})
	r.put("update holiday", "/api/holidays/"+s.holiday, jsonObject{"notes": "Scenario, clinic closed"}, http.StatusOK)
	tempHoliday := r.create("create holiday to delete", "/api/holidays",
		jsonObject{"name": "Test Temp Holiday", "startDate": "2025-11-10", "endDate": "2025-11-11"})
	r.del("delete holiday", "/api/holidays/"+tempHoliday, http.StatusOK)
}

func (r *contractRun) people(s *contractScenario) {
	t := r.t
	s.ada = r.create("create patient Ada", "/api/patients", jsonObject{"firstName": "Ada", "lastName": "Example", "gender": "Female",
		"dateOfBirth": "1989-04-12", "contact": "70 123 456", "email": "ada@example.test",
		"emergencyContactName": "Ben Sample", "emergencyContactPhone": "70123457", "weight": 61.5, "height": 168,
		"bloodType": "A+", "countryId": seedLebanon, "cityId": seedBeirut, "address": "12 Sample Street",
		"referralSource": "Instagram", "notes": "Scenario patient"})
	s.ben = r.create("create patient Ben", "/api/patients", jsonObject{"firstName": "Ben", "lastName": "Sample", "gender": "Male",
		"dateOfBirth": "2004-07-01", "contact": "71123456", "countryId": seedLebanon, "cityId": seedJounieh, "referralSource": "Friend"})
	s.cleo = r.create("create patient Cleo", "/api/patients", jsonObject{"firstName": "Cleo", "lastName": "Placeholder", "gender": "Female",
		"dateOfBirth": "1965-02-20", "contact": "76123456", "countryId": seedLebanon, "cityId": seedBeirut,
		"referralId": s.ada, "referralSource": "Referral"})
	dan := r.create("create patient to delete", "/api/patients", jsonObject{"firstName": "Dan", "lastName": "Dummy", "gender": "Male",
		"dateOfBirth": "1995-05-05", "contact": "78123456"})
	r.put("update patient Ada", "/api/patients/"+s.ada, jsonObject{"middleName": "Q", "notes": "Prefers mornings"}, http.StatusOK)
	r.del("delete patient without records", "/api/patients/"+dan, http.StatusOK)

	s.adaLatex = r.create("add patient allergy", "/api/patients/"+s.ada+"/allergies", jsonObject{"allergyId": s.latex, "notes": "Rash"})
	r.put("update patient allergy", "/api/patient-allergies/"+s.adaLatex, jsonObject{"notes": "Severe rash"}, http.StatusOK)
	benPollen := r.create("add patient allergy to remove", "/api/patients/"+s.ben+"/allergies", jsonObject{"allergyId": s.pollen})
	r.del("remove patient allergy", "/api/patient-allergies/"+benPollen, http.StatusOK)

	s.adaAmoxi = r.create("add patient medicine", "/api/patients/"+s.ada+"/medicines",
		jsonObject{"medicineId": s.amoxi, "isActive": true, "notes": "Twice daily"})
	r.put("update patient medicine", "/api/patient-medicines/"+s.adaAmoxi, jsonObject{"notes": "Twice daily, with food"}, http.StatusOK)
	benIbu := r.create("add patient medicine to remove", "/api/patients/"+s.ben+"/medicines", jsonObject{"medicineId": s.ibu, "isActive": true})
	r.del("remove patient medicine", "/api/patient-medicines/"+benIbu, http.StatusOK)

	var emp struct {
		ID     string `json:"id"`
		UserID string `json:"userId"`
	}
	nora := jsonObject{"firstName": "Nora", "lastName": "Example", "role": "Nurse", "contact": "70111222", "email": "nora@example.test",
		"dateOfBirth": "1992-09-09", "employmentType": "Full-time", "username": "nora.example", "password": "nora-pw", "userRole": "nurse"}
	r.post("create employee Nora with a nurse account", "/api/employees", nora, http.StatusCreated).data(t, &emp)
	s.nora, s.noraUser = emp.ID, emp.UserID
	sam := jsonObject{"firstName": "Sam", "lastName": "Sample", "role": "Receptionist", "contact": "70111333",
		"employmentType": "Part-time", "username": "sam.sample", "password": "sam-pw", "userRole": "staff"}
	r.post("create employee Sam with a staff account", "/api/employees", sam, http.StatusCreated).data(t, &emp)
	s.sam, s.samUser = emp.ID, emp.UserID
	s.otto = r.create("create employee Otto without an account", "/api/employees", jsonObject{"firstName": "Otto", "lastName": "Placeholder",
		"role": "Doctor", "contact": "70111444", "employmentType": "Full-time"})
	r.put("update employee Otto", "/api/employees/"+s.otto, jsonObject{"email": "otto@example.test"}, http.StatusOK)
	temp := r.create("create employee to delete", "/api/employees", jsonObject{"firstName": "Temp", "lastName": "Worker",
		"role": "Cleaner", "contact": "70111555", "employmentType": "Part-time"})
	r.del("delete employee without records", "/api/employees/"+temp, http.StatusOK)

	s.nurse = r.login("nurse", "nora.example", "nora-pw")
	s.staff = r.login("staff", "sam.sample", "sam-pw")

	r.put("rename user", "/api/users/"+s.retiredUser, jsonObject{"displayName": "Retired Example Person"}, http.StatusOK)
	r.put("deactivate user", "/api/users/"+s.retiredUser, jsonObject{"isActive": false}, http.StatusOK)
	r.put("change a user's role", "/api/users/"+s.noPassUser, jsonObject{"role": "staff"}, http.StatusOK)
	r.put("update role label", "/api/roles/nurse", jsonObject{"label": "Nurse"}, http.StatusOK)

	// The staff role gains employees:write for a moment: creating an employee
	// with an account still needs users:write.
	wider := append(slices.Clone(s.staffScopes), "employees:write")
	r.put("let staff create employees", "/api/roles/staff", jsonObject{"scopes": wider}, http.StatusOK)
	r.run(contractCall{name: "staff creates an employee with an account", method: http.MethodPost, path: "/api/employees", asCaller: s.staff,
		body: jsonObject{"firstName": "Blocked", "lastName": "Account", "role": "Nurse", "contact": "70111666",
			"employmentType": "Full-time", "username": "blocked.account", "password": "x-pw"}})
	r.put("restore the staff role", "/api/roles/staff", jsonObject{"scopes": s.staffScopes}, http.StatusOK)

	s.adaRx = r.create("create prescription", "/api/prescriptions", jsonObject{"patientId": s.ada, "prescribedById": s.otto,
		"startDate": "2025-03-31", "endDate": "2025-04-14", "medicines": []jsonObject{
			{"medicineId": s.amoxi, "instructions": "1 capsule twice daily"},
			{"medicineId": s.ibu, "instructions": "As needed"}}})
	r.put("update prescription", "/api/prescriptions/"+s.adaRx, jsonObject{"patientId": s.ada, "prescribedById": s.otto,
		"startDate": "2025-03-31", "endDate": "2025-04-21", "medicines": []jsonObject{
			{"medicineId": s.amoxi, "instructions": "1 capsule three times daily"}}}, http.StatusOK)
	benRx := r.create("create prescription to delete", "/api/prescriptions", jsonObject{"patientId": s.ben, "prescribedById": s.otto,
		"startDate": "2025-04-01", "medicines": []jsonObject{{"medicineId": s.ibu, "instructions": "Once"}}})
	r.del("delete prescription", "/api/prescriptions/"+benRx, http.StatusOK)
	r.run(contractCall{name: "nurse creates a prescription", method: http.MethodPost, path: "/api/prescriptions", asCaller: s.nurse,
		want: http.StatusCreated, body: jsonObject{"patientId": s.cleo, "prescribedById": s.nora, "startDate": "2025-10-27",
			"medicines": []jsonObject{{"medicineId": s.ibu, "instructions": "After meals"}}}})
}

func (r *contractRun) hr(s *contractScenario) {
	t := r.t
	var shifts []struct {
		ID string `json:"id"`
	}
	r.put("save Nora's Monday schedule", "/api/employee-schedules/day", jsonObject{"employeeId": s.nora, "dayOfWeek": 1,
		"startDate": "2025-03-03", "shifts": []jsonObject{{"startTime": "09:00", "endTime": "13:00"}, {"startTime": "14:00", "endTime": "18:00"}}},
		http.StatusOK).data(t, &shifts)
	s.noraShift = shifts[0].ID
	r.put("save Nora's Saturday schedule", "/api/employee-schedules/day", jsonObject{"employeeId": s.nora, "dayOfWeek": 6,
		"startDate": "2025-03-03", "shifts": []jsonObject{{"startTime": "09:00", "endTime": "14:00"}}}, http.StatusOK)
	r.put("save Nora's Sunday schedule", "/api/employee-schedules/day", jsonObject{"employeeId": s.nora, "dayOfWeek": 0,
		"startDate": "2025-03-02", "shifts": []jsonObject{{"startTime": "08:00", "endTime": "12:00"}}}, http.StatusOK)
	r.put("start a new version of Nora's Monday", "/api/employee-schedules/day", jsonObject{"employeeId": s.nora, "dayOfWeek": 1,
		"startDate": "2025-04-07", "shifts": []jsonObject{{"startTime": "10:00", "endTime": "16:00"}}}, http.StatusOK)
	r.put("save Sam's Tuesday schedule", "/api/employee-schedules/day", jsonObject{"employeeId": s.sam, "dayOfWeek": 2,
		"startDate": "2025-03-04", "shifts": []jsonObject{{"startTime": "12:00", "endTime": "20:00"}}}, http.StatusOK).data(t, &shifts)
	s.samShift = shifts[0].ID
	r.put("save Sam's Wednesday schedule", "/api/employee-schedules/day", jsonObject{"employeeId": s.sam, "dayOfWeek": 3,
		"startDate": "2025-03-05", "shifts": []jsonObject{{"startTime": "12:00", "endTime": "16:00"}}}, http.StatusOK).data(t, &shifts)
	r.del("delete a schedule version", "/api/employee-schedules/"+shifts[0].ID, http.StatusOK)

	s.timeoff = r.create("request time off", "/api/employee-schedule-changes", jsonObject{"employeeId": s.nora, "type": "timeoff",
		"startDate": "2025-03-31", "endDate": "2025-03-31", "startTime": "14:00", "endTime": "18:00", "notes": "Dentist"})
	r.put("update time off", "/api/employee-schedule-changes/"+s.timeoff, jsonObject{"notes": "Dentist appointment"}, http.StatusOK)
	r.post("accept time off", "/api/employee-schedule-changes/"+s.timeoff+"/status", jsonObject{"status": "accepted"}, http.StatusOK)
	s.overtime = r.create("request overtime", "/api/employee-schedule-changes", jsonObject{"employeeId": s.nora, "type": "overtime",
		"startDate": "2025-04-01", "endDate": "2025-04-01", "startTime": "18:00", "endTime": "20:00"})
	r.post("accept overtime", "/api/employee-schedule-changes/"+s.overtime+"/status", jsonObject{"status": "accepted"}, http.StatusOK)
	own := r.run(contractCall{name: "staff requests time off for someone else", method: http.MethodPost, path: "/api/employee-schedule-changes",
		asCaller: s.staff, want: http.StatusCreated, body: jsonObject{"employeeId": s.nora, "type": "timeoff", "startDate": "2025-10-27",
			"endDate": "2025-10-28"}}).id(t)
	r.post("reject time off", "/api/employee-schedule-changes/"+own+"/status", jsonObject{"status": "rejected"}, http.StatusOK)
	r.run(contractCall{name: "staff without an employee requests time off", method: http.MethodPost, path: "/api/employee-schedule-changes",
		asCaller: s.unlinked, body: jsonObject{"type": "timeoff", "startDate": "2025-10-27", "endDate": "2025-10-27"}})
	drop := r.create("request time off to delete", "/api/employee-schedule-changes", jsonObject{"employeeId": s.sam, "type": "timeoff",
		"startDate": "2025-04-02", "endDate": "2025-04-03"})
	r.del("delete schedule change", "/api/employee-schedule-changes/"+drop, http.StatusOK)

	s.noraSalary = r.create("set Nora's salary", "/api/employees/"+s.nora+"/salaries",
		jsonObject{"amount": 1200, "currencyId": "USD", "effectiveDate": "2025-01-01", "notes": "Base"})
	r.create("set Sam's salary", "/api/employees/"+s.sam+"/salaries", jsonObject{"amount": 900, "effectiveDate": "2025-02-01"})
	r.put("raise Nora's salary", "/api/employee-salaries/"+s.noraSalary, jsonObject{"amount": 1250}, http.StatusOK)
	ottoSalary := r.create("set a salary to delete", "/api/employees/"+s.otto+"/salaries", jsonObject{"amount": 2000, "effectiveDate": "2025-01-01"})
	r.del("delete salary", "/api/employee-salaries/"+ottoSalary, http.StatusOK)

	var preps []struct {
		ID         string `json:"id"`
		EmployeeID string `json:"employeeId"`
	}
	r.post("prepare March salaries", "/api/employee-salaries/prepare",
		jsonObject{"periodStart": "2025-03-01", "periodEnd": "2025-03-31", "notes": "March"}, http.StatusCreated).data(t, &preps)
	for _, p := range preps {
		if p.EmployeeID == s.nora {
			s.noraPrep = p.ID
		}
	}
	r.run(contractCall{name: "adjust Nora's March salary", method: http.MethodPatch, path: "/api/employee-salary-preparations/" + s.noraPrep,
		body: jsonObject{"adjustment": 50}, want: http.StatusOK})
	r.post("prepare April salaries", "/api/employee-salaries/prepare",
		jsonObject{"periodStart": "2025-04-01", "periodEnd": "2025-04-30"}, http.StatusCreated).data(t, &preps)
	for _, p := range preps {
		if p.EmployeeID == s.sam {
			r.del("delete Sam's April preparation", "/api/employee-salary-preparations/"+p.ID, http.StatusOK)
		}
	}
}

func (r *contractRun) appointments(s *contractScenario) {
	t := r.t
	appt := func(name, patient, room, date, from, to, notes string, procs []jsonObject) string {
		return r.create(name, "/api/appointments", jsonObject{"patientId": patient, "roomId": room,
			"startTime": clinicUTC(t, date, from), "endTime": clinicUTC(t, date, to), "notes": notes,
			"appointmentProcedures": procs})
	}
	s.apptDST1 = appt("book the Saturday before DST starts", s.ada, seedRoom1, "2025-03-29", "10:00", "11:00", "Before DST",
		[]jsonObject{{"procedureId": s.peel, "assignedToId": s.nora, "notes": "First session"},
			{"procedureId": s.laser, "assignedToId": s.nora}})
	s.apptDST2 = appt("book the Monday after DST starts", s.ben, seedRoom1, "2025-03-31", "10:00", "10:30", "After DST",
		[]jsonObject{{"procedureId": s.peel}})
	s.apptLate = appt("book just after clinic midnight", s.cleo, seedRoom2, "2025-03-31", "00:30", "01:00", "Late slot",
		[]jsonObject{{"procedureId": s.laser, "assignedToId": s.nora}})
	s.apptCancelled = appt("book the Saturday before DST ends", s.ada, seedRoom1, "2025-10-25", "10:00", "11:00", "",
		[]jsonObject{{"procedureId": s.peel}})
	s.apptSun = appt("book the Sunday DST ends", s.cleo, seedRoom3, "2025-10-26", "09:00", "09:45", "Fall back", nil)
	s.apptHoliday = appt("book the holiday after DST ends", s.ben, s.room, "2025-10-27", "10:00", "11:00", "On a holiday",
		[]jsonObject{{"procedureId": s.peel, "assignedToId": s.nora}, {"procedureId": s.laser, "assignedToId": s.sam, "notes": "Assist"}})

	var moved struct {
		ID string `json:"id"`
	}
	r.post("reschedule the Monday appointment", "/api/appointments/"+s.apptDST2+"/reschedule", jsonObject{"roomId": seedRoom2,
		"startTime": clinicUTC(t, "2025-03-31", "12:00"), "endTime": clinicUTC(t, "2025-03-31", "12:30"),
		"cancelNotes": "Patient asked"}, http.StatusCreated).data(t, &moved)
	s.apptMoved = moved.ID
	r.put("complete the late appointment", "/api/appointments/"+s.apptLate,
		jsonObject{"status": "Completed", "completionNotes": "Went well"}, http.StatusOK)
	r.put("cancel an appointment", "/api/appointments/"+s.apptCancelled,
		jsonObject{"status": "Cancelled", "cancelNotes": "Patient ill"}, http.StatusOK)
	r.put("start the holiday appointment", "/api/appointments/"+s.apptHoliday,
		jsonObject{"status": "In-Progress", "appointmentProcedures": []jsonObject{{"procedureId": s.peel, "assignedToId": s.nora, "notes": "Swapped"}}},
		http.StatusOK)
	r.del("delete an appointment", "/api/appointments/"+s.apptSun, http.StatusOK)

	// Relative to the run's day, at fixed UTC times.
	s.apptPast = r.create("book three days ago", "/api/appointments", jsonObject{"patientId": s.ada, "roomId": seedRoom1,
		"startTime": r.day(-3) + "T07:00:00.000Z", "endTime": r.day(-3) + "T08:00:00.000Z", "status": "Completed",
		"appointmentProcedures": []jsonObject{{"procedureId": s.peel, "assignedToId": s.nora}}})
	s.apptSoon = r.run(contractCall{name: "staff books in two days", method: http.MethodPost, path: "/api/appointments", asCaller: s.staff,
		want: http.StatusCreated, body: jsonObject{"patientId": s.ben, "roomId": seedRoom2,
			"startTime": r.day(2) + "T07:00:00.000Z", "endTime": r.day(2) + "T07:30:00.000Z",
			"appointmentProcedures": []jsonObject{{"procedureId": s.laser, "assignedToId": s.nora}}}}).id(t)
}

func (r *contractRun) finance(s *contractScenario) {
	t := r.t
	// Supplier invoices bring stock in.
	var inv struct {
		ID            string `json:"id"`
		InvoiceNumber int    `json:"invoiceNumber"`
		Items         []struct {
			ID       string `json:"id"`
			ItemType string `json:"itemType"`
			ItemID   string `json:"itemId"`
		} `json:"items"`
	}
	r.post("receive serum from Acme", "/api/supplier-invoices", jsonObject{"supplierId": s.acme, "notes": "First delivery",
		"items": []jsonObject{{"itemType": "product", "itemId": s.serum, "quantity": 10, "amount": 200},
			{"itemType": "other", "quantity": 1, "amount": 15, "notes": "Delivery fee"}}}, http.StatusCreated).data(t, &inv)
	s.supplierInvoice = inv.ID
	for _, it := range inv.Items {
		if it.ItemType == "product" {
			s.serumItem = it.ID
		}
	}
	r.put("update supplier invoice", "/api/supplier-invoices/"+s.supplierInvoice, jsonObject{"notes": "First delivery, checked"}, http.StatusOK)
	r.put("correct a supplier invoice line", "/api/supplier-invoices/"+s.supplierInvoice+"/items/"+s.serumItem,
		jsonObject{"amount": 180}, http.StatusOK)
	globexInv := r.run(contractCall{name: "admin receives cream from Globex", method: http.MethodPost, path: "/api/supplier-invoices",
		asCaller: s.admin, want: http.StatusCreated, body: jsonObject{"supplierId": s.globex,
			"items": []jsonObject{{"itemType": "product", "itemId": s.cream, "quantity": 5, "amount": 90}}}}).id(t)

	// Client invoices with all four line types, an offer and gift cards.
	s.giftCode = "TESTGIFT-1"
	r.post("invoice Ada", "/api/client-invoices", jsonObject{"patientId": s.ada, "discountId": s.spring, "notes": "Visit",
		"items": []jsonObject{
			{"itemType": "procedure", "itemId": s.peel, "quantity": 1, "amount": 160},
			{"itemType": "product", "itemId": s.cream, "quantity": 2, "amount": 55},
			{"itemType": "other", "quantity": 1, "amount": 30, "notes": "Consultation fee"},
			{"itemType": "gift", "quantity": 1, "amount": 100, "giftCode": s.giftCode, "giftName": "Test Gift Card"}}},
		http.StatusCreated).data(t, &inv)
	s.invoice1 = inv.ID
	for _, it := range inv.Items {
		if it.ItemType == "gift" {
			s.giftID = it.ItemID
		}
	}
	s.invoice2 = r.create("invoice Ben with a gift for Cleo", "/api/client-invoices", jsonObject{"patientId": s.ben,
		"items": []jsonObject{
			{"itemType": "procedure", "itemId": s.laser, "quantity": 1, "amount": 300},
			{"itemType": "gift", "quantity": 1, "amount": 50, "giftPatientId": s.cleo, "giftName": "Cleo Gift"}}})
	cleoInv := r.create("invoice Cleo", "/api/client-invoices", jsonObject{"patientId": s.cleo, "discountId": s.fixedOffer,
		"items": []jsonObject{{"itemType": "product", "itemId": s.cream, "quantity": 1, "amount": 27.5}}})
	r.put("update client invoice", "/api/client-invoices/"+s.invoice1, jsonObject{"notes": "Visit, paid in two parts"}, http.StatusOK)
	r.del("delete client invoice", "/api/client-invoices/"+cleoInv, http.StatusOK)
	r.del("delete supplier invoice", "/api/supplier-invoices/"+globexInv, http.StatusOK)
	r.run(contractCall{name: "staff invoices Ben for serum", method: http.MethodPost, path: "/api/client-invoices", asCaller: s.staff,
		want: http.StatusCreated, body: jsonObject{"patientId": s.ben, "items": []jsonObject{
			{"itemType": "product", "itemId": s.serum, "quantity": 8, "amount": 320}}}})
	r.post("redeem gift card", "/api/gift-cards/redeem", jsonObject{"code": s.giftCode, "patientId": s.ben}, http.StatusOK)
	r.put("deactivate fixed offer", "/api/discounts/"+s.fixedOffer, jsonObject{"isActive": 0}, http.StatusOK)

	// Client payments, refunds, adjustments and write-offs.
	s.adaPayment = r.create("Ada pays a deposit", "/api/client-payments",
		jsonObject{"patientId": s.ada, "amount": 200, "transactionMethod": "cash", "description": "Deposit"})
	r.create("Ada pays by card", "/api/client-payments", jsonObject{"patientId": s.ada, "amount": 50, "transactionMethod": "card"})
	r.create("refund Ada", "/api/client-refunds", jsonObject{"patientId": s.ada, "amount": 20, "transactionMethod": "cash", "description": "Overcharge"})
	r.create("adjust Ada's balance", "/api/client-adjustments", jsonObject{"patientId": s.ada, "amount": 5, "transactionMethod": "other",
		"direction": "incoming", "description": "Rounding"})
	r.create("write off Ben's balance", "/api/client-write-offs", jsonObject{"patientId": s.ben, "amount": 10, "direction": "incoming",
		"description": "Small balance"})
	benPay := r.run(contractCall{name: "staff records Ben's payment", method: http.MethodPost, path: "/api/client-payments", asCaller: s.staff,
		want: http.StatusCreated, body: jsonObject{"patientId": s.ben, "amount": 15, "transactionMethod": "transfer"}}).id(t)
	r.del("delete client payment", "/api/client-payments/"+benPay, http.StatusOK)

	// Supplier payments.
	s.acmePayment = r.create("pay Acme", "/api/supplier-payments",
		jsonObject{"supplierId": s.acme, "amount": 150, "transactionMethod": "transfer", "description": "Partial"})
	r.create("adjust Acme's balance", "/api/supplier-adjustments", jsonObject{"supplierId": s.acme, "amount": 5, "transactionMethod": "other",
		"direction": "outgoing", "description": "Credit note"})
	r.create("write off Acme's balance", "/api/supplier-write-offs", jsonObject{"supplierId": s.acme, "amount": 2, "direction": "incoming",
		"description": "Rounding"})
	acmeExtra := r.create("pay Acme again", "/api/supplier-payments", jsonObject{"supplierId": s.acme, "amount": 10, "transactionMethod": "cash"})
	r.del("delete supplier payment", "/api/supplier-payments/"+acmeExtra, http.StatusOK)

	// Expense payments.
	s.rentPayment = r.create("pay rent", "/api/expense-payments",
		jsonObject{"expenseId": s.rent, "amount": 500, "transactionMethod": "transfer", "description": "March rent"})
	r.create("adjust rent", "/api/expense-adjustments", jsonObject{"expenseId": s.rent, "amount": 20, "transactionMethod": "other",
		"direction": "incoming", "description": "Late fee"})
	r.create("write off utilities", "/api/expense-write-offs", jsonObject{"expenseId": s.utilities, "amount": 5, "direction": "outgoing",
		"description": "Waived"})
	utilPay := r.create("pay utilities", "/api/expense-payments", jsonObject{"expenseId": s.utilities, "amount": 80, "transactionMethod": "cash"})
	r.del("delete expense payment", "/api/expense-payments/"+utilPay, http.StatusOK)

	// Employee payments.
	s.noraPayment = r.create("pay Nora", "/api/employee-payments",
		jsonObject{"employeeId": s.nora, "amount": 600, "transactionMethod": "transfer", "description": "March advance"})
	r.create("adjust Nora's balance", "/api/employee-adjustments", jsonObject{"employeeId": s.nora, "amount": 25, "transactionMethod": "other",
		"direction": "outgoing", "description": "Bonus"})
	r.create("write off Sam's balance", "/api/employee-write-offs", jsonObject{"employeeId": s.sam, "amount": 10, "direction": "incoming",
		"description": "Uniform"})
	samPay := r.create("pay Sam", "/api/employee-payments", jsonObject{"employeeId": s.sam, "amount": 100, "transactionMethod": "cash"})
	r.del("delete employee payment", "/api/employee-payments/"+samPay, http.StatusOK)
}

func (r *contractRun) notifications(s *contractScenario) {
	t := r.t
	s.notifRead = r.post("send a test notification", "/api/notifications/test", nil, http.StatusOK).id(t)
	second := r.post("send another test notification", "/api/notifications/test", nil, http.StatusOK).id(t)
	r.put("mark a notification read", "/api/notifications/"+s.notifRead+"/read", nil, http.StatusOK)
	r.del("delete a notification", "/api/notifications/"+second, http.StatusOK)
	r.put("mark all notifications read", "/api/notifications/read-all", nil, http.StatusOK)
	s.nurseNotif = r.run(contractCall{name: "nurse sends a test notification", method: http.MethodPost, path: "/api/notifications/test",
		asCaller: s.nurse, want: http.StatusOK}).id(t)

	// Last, so the name each record copied at its creation shows next to the new one.
	r.put("rename patient Cleo", "/api/patients/"+s.cleo, jsonObject{"lastName": "Example"}, http.StatusOK)
}
