package store

import (
	"errors"
	"testing"
)

// The constraint translation runs against real writes, so the driver's own
// error shape passes through database/sql the way it does in the server.

func TestConstraintError_UniqueClashBecomesConflict(t *testing.T) {
	setupTestDB(t)
	first := User{Username: "taken", DisplayName: "First", Role: "staff", IsActive: true}
	if err := first.Create("h1"); err != nil {
		t.Fatal(err)
	}
	second := User{Username: "taken", DisplayName: "Second", Role: "staff", IsActive: true}
	err := second.Create("h2")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate username: got %v, want ErrConflict", err)
	}
	if got, want := err.Error(), "conflict: Username is already taken"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestConstraintError_MissingParentBecomesValidation(t *testing.T) {
	setupTestDB(t)
	s := EmployeeSalary{
		EmployeeID:    "00000000-0000-7000-8000-000000000000",
		Amount:        100,
		EffectiveDate: "2025-01-01",
	}
	err := s.Create()
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("unknown employee: got %v, want ErrValidation", err)
	}
	if got, want := err.Error(), "validation: Related record not found"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestPatientMedicine_Create_DuplicateIsConflict(t *testing.T) {
	setupTestDB(t)
	p := makePatient(t, "Duplicatemed", "Patient", "70010001")
	m := Medicine{Name: "Duplicatemed pills"}
	if err := m.Create(); err != nil {
		t.Fatal(err)
	}
	first := PatientMedicine{PatientID: p.ID, MedicineID: m.ID, IsActive: true}
	if err := first.Create(); err != nil {
		t.Fatal(err)
	}
	second := PatientMedicine{PatientID: p.ID, MedicineID: m.ID}
	err := second.Create()
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("same medicine twice: got %v, want ErrConflict", err)
	}
	if got, want := err.Error(), "conflict: Patient already takes this medicine"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestAllergy_Create_DuplicateNameIsConflict(t *testing.T) {
	setupTestDB(t)
	first := Allergy{Name: "Pollen"}
	if err := first.Create(); err != nil {
		t.Fatal(err)
	}
	second := Allergy{Name: "Pollen"}
	err := second.Create()
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("same allergy name twice: got %v, want ErrConflict", err)
	}
	if got, want := err.Error(), "conflict: An allergy with this name already exists"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestCurrency_Create_DuplicateCodeIsConflict(t *testing.T) {
	setupTestDB(t)
	first := Currency{Code: "TST", Name: "Test", Symbol: "T"}
	if err := first.Create(); err != nil {
		t.Fatal(err)
	}
	second := Currency{Code: "TST", Name: "Other", Symbol: "O"}
	err := second.Create()
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("same currency code twice: got %v, want ErrConflict", err)
	}
	if got, want := err.Error(), "conflict: A currency with this code already exists"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestDiscount_Create_DuplicateCodeIsConflict(t *testing.T) {
	setupTestDB(t)
	code := "TAKE10"
	first := Discount{Name: "Ten off", DiscountType: "gift", ValueType: "fixed", Value: 10, Code: &code}
	if err := first.Create(); err != nil {
		t.Fatal(err)
	}
	second := Discount{Name: "Ten off again", DiscountType: "gift", ValueType: "fixed", Value: 5, Code: &code}
	err := second.Create()
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("same discount code twice: got %v, want ErrConflict", err)
	}
	if got, want := err.Error(), "conflict: This code is already in use"; got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestPrescription_Create_MissingParentsAreValidation(t *testing.T) {
	setupTestDB(t)
	patient := makePatient(t, "Rxtest", "Patient", "70010002")
	prescriber := makeEmployee(t, "Rxtest", "Doctor")
	unknown := "00000000-0000-7000-8000-000000000000"

	cases := []struct {
		name string
		body Prescription
	}{
		{"unknown patient", Prescription{PatientID: unknown, PrescribedByID: prescriber.ID, StartDate: "2025-01-01"}},
		{"unknown prescriber", Prescription{PatientID: patient.ID, PrescribedByID: unknown, StartDate: "2025-01-01"}},
		{"unknown medicine", Prescription{PatientID: patient.ID, PrescribedByID: prescriber.ID, StartDate: "2025-01-01",
			Medicines: []PrescriptionMedicine{{MedicineID: unknown}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.body.Create()
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("got %v, want ErrValidation", err)
			}
			if got, want := err.Error(), "validation: Related record not found"; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
}

func TestAppointment_Create_UnknownProcedureOrAssigneeIsValidation(t *testing.T) {
	setupTestDB(t)
	patient := makePatient(t, "Procunknown", "Patient", "70010003")
	room := Room{Name: "Procunknown room", Type: "Procedure"}
	if err := room.Create(); err != nil {
		t.Fatal(err)
	}
	procedure := Procedure{Name: "Procunknown check"}
	if err := procedure.Create(); err != nil {
		t.Fatal(err)
	}
	unknown := "00000000-0000-7000-8000-000000000000"

	cases := []struct {
		name string
		link AppointmentProcedure
	}{
		{"unknown procedure", AppointmentProcedure{ProcedureID: unknown}},
		{"unknown assignee", AppointmentProcedure{ProcedureID: procedure.ID, AssignedToID: unknown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apt := Appointment{
				PatientID: patient.ID, RoomID: room.ID,
				StartTime: "2025-06-02T10:00:00Z", EndTime: "2025-06-02T11:00:00Z",
				Status:                "Scheduled",
				AppointmentProcedures: []AppointmentProcedure{tc.link},
			}
			err := apt.Create()
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("got %v, want ErrValidation", err)
			}
			if got, want := err.Error(), "validation: Related record not found"; got != want {
				t.Errorf("message = %q, want %q", got, want)
			}
		})
	}
}

func TestHoliday_Create_RulesAreValidationErrors(t *testing.T) {
	setupTestDB(t)
	cases := []struct {
		name string
		h    Holiday
		want string
	}{
		{"without a name", Holiday{StartDate: "2025-05-02", EndDate: "2025-05-02"}, "validation: Name is required"},
		{"without dates", Holiday{Name: "No dates"}, "validation: Start and end dates are required"},
		{"with a malformed start date", Holiday{Name: "Malformed", StartDate: "sometime", EndDate: "2025-05-02"}, "validation: Start date must be a valid date"},
		{"with a malformed end date", Holiday{Name: "Malformed", StartDate: "2025-05-02", EndDate: "later"}, "validation: End date must be a valid date"},
		{"ending before it starts", Holiday{Name: "Backwards", StartDate: "2025-05-02", EndDate: "2025-05-01"}, "validation: End date must be on or after start date"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.h.Create()
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("got %v, want ErrValidation", err)
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("message = %q, want %q", got, tc.want)
			}
		})
	}
}
