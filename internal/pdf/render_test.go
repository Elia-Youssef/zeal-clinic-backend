package pdf

import (
	"fmt"
	"testing"

	"clinic-api/internal/database/store"
)

// TestRenderPDFs is a layout smoke test: it renders each document with
// representative data and asserts generation succeeds. maroto can panic or
// error on bad column geometry at Generate() time, which build/vet won't catch.
// The PDFs land under the working directory, so the test runs in a temp dir.
func TestRenderPDFs(t *testing.T) {
	t.Chdir(t.TempDir())

	inv := &store.Invoice{
		ID: "inv1", InvoiceNumber: 42, ToEntityName: "Test Patient",
		Amount: 300, DiscountValue: 50, FinalAmount: 250, CreatedAt: store.Date("2026-05-20"),
		Items: []store.InvoiceItem{
			{ID: "i1", ItemType: "procedure", ItemID: "p1", Quantity: 1, Amount: 200, FinalAmount: 200, ItemName: "Botox Full"},
			{ID: "i2", ItemType: "product", ItemID: "pr1", Quantity: 2, Amount: 100, FinalAmount: 100, ItemName: "Vitamin C Serum"},
		},
	}
	if p, err := GenerateInvoice(inv); err != nil || p == "" {
		t.Fatalf("invoice: path=%q err=%v", p, err)
	}

	rev := &store.RevenueReport{
		Level:  "procedure",
		Totals: store.RevenueTotals{Quantity: 3, Amount: 770},
		Items: []store.RevenueGroup{
			{EntityName: "Botox Full", Quantity: 2, Amount: 440, Percentage: 57.1},
			{EntityName: "Lips", Quantity: 1, Amount: 330, Percentage: 42.9},
		},
	}
	if p, err := GenerateRevenueReport(rev, "2026-05-01", "2026-05-31"); err != nil || p == "" {
		t.Fatalf("revenue: path=%q err=%v", p, err)
	}

	exp := store.ExpensesReport{
		Rows: []store.ExpenseRow{
			{Date: store.Date("2026-05-10"), Supplier: "Cobalt Clinical Supplies", Description: "Botox stock", Quantity: 4, Amount: 840, RemainingBalance: 0},
		},
		Totals: store.ExpensesTotals{Amount: 840},
	}
	if p, err := GenerateExpensesReport(exp, "2026-05-01", "2026-05-31"); err != nil || p == "" {
		t.Fatalf("expenses: path=%q err=%v", p, err)
	}

	apts := store.AppointmentList{
		{
			PatientName: "Test Patient", RoomID: "r1", Status: "Completed",
			StartTime: store.Date("2026-05-20T10:00:00Z"), EndTime: store.Date("2026-05-20T11:00:00Z"),
			Notes:                 "Bring previous lab results",
			AppointmentProcedures: []store.AppointmentProcedure{{ProcedureName: "Botox Full"}},
		},
		{
			PatientName: "Cancelled Patient", RoomID: "r1", Status: "Cancelled",
			StartTime: store.Date("2026-05-20T11:00:00Z"), EndTime: store.Date("2026-05-20T12:00:00Z"),
			Notes: "Bring previous lab results", CancelNotes: "Patient called in sick",
			AppointmentProcedures: []store.AppointmentProcedure{{ProcedureName: "Lips"}},
		},
	}
	if p, err := GenerateAppointments(apts, map[string]string{"r1": "Room 1"}, "2026-05-20"); err != nil || p == "" {
		t.Fatalf("appointments: path=%q err=%v", p, err)
	}

	// Multi-page: many rows force pagination, exercising the repeating header.
	big := store.AppointmentList{}
	for i := 0; i < 120; i++ {
		big = append(big, store.Appointment{
			PatientName: fmt.Sprintf("Patient %d", i), RoomID: "r1", Status: "Completed",
			StartTime: store.Date("2026-05-20T10:00:00Z"), EndTime: store.Date("2026-05-20T11:00:00Z"),
			AppointmentProcedures: []store.AppointmentProcedure{{ProcedureName: "Botox Full"}},
		})
	}
	if p, err := GenerateAppointments(big, map[string]string{"r1": "Room 1"}, "2026-05-20"); err != nil || p == "" {
		t.Fatalf("multi-page appointments: path=%q err=%v", p, err)
	}
}
