package store

import (
	"clinic-api/internal/validation"
	"errors"
	"testing"
)

func TestDiscount_Create_ValidateIsActive(t *testing.T) {
	setupTestDB(t)

	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name       string
		isActive   *int
		wantStored int
		wantErr    bool
	}{
		{name: "absent defaults to 1", isActive: nil, wantStored: 1},
		{name: "0 is stored", isActive: intPtr(0), wantStored: 0},
		{name: "1 is stored", isActive: intPtr(1), wantStored: 1},
		{name: "2 is rejected", isActive: intPtr(2), wantErr: true},
		{name: "-1 is rejected", isActive: intPtr(-1), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Discount{
				Name:         "Create Offer " + tc.name,
				DiscountType: "offer",
				ValueType:    "percentage",
				Value:        10,
				IsActive:     tc.isActive,
			}
			err := d.Create()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for isActive %v, got nil", tc.isActive)
				}
				var valErr validation.Errors
				if !errors.As(err, &valErr) {
					t.Fatalf("expected validation.Errors, got %v", err)
				}
				if _, ok := valErr["isActive"]; !ok {
					t.Errorf("expected error key 'isActive', got %v", valErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}
			var stored Discount
			if err := stored.GetByID(d.ID); err != nil {
				t.Fatalf("get: %v", err)
			}
			if stored.IsActive == nil || *stored.IsActive != tc.wantStored {
				t.Errorf("stored isActive = %v, want %d", stored.IsActive, tc.wantStored)
			}
		})
	}
}

func TestDiscount_Update_ValidateFields(t *testing.T) {
	setupTestDB(t)

	tests := []struct {
		name       string
		field      string
		value      any
		wantErrKey string
		wantStored int
	}{
		{name: "0 is stored", field: "isActive", value: 0, wantStored: 0},
		{name: "1 is stored", field: "isActive", value: 1, wantStored: 1},
		{name: "1.0 is stored", field: "isActive", value: float64(1), wantStored: 1},
		{name: "2 is rejected", field: "isActive", value: 2, wantErrKey: "isActive"},
		{name: "-1 is rejected", field: "isActive", value: -1, wantErrKey: "isActive"},
		{name: "0.5 is rejected", field: "isActive", value: 0.5, wantErrKey: "isActive"},
		{name: "1.5 is rejected", field: "isActive", value: 1.5, wantErrKey: "isActive"},
		{name: "null is rejected", field: "isActive", value: nil, wantErrKey: "isActive"},
		{name: "string is rejected", field: "isActive", value: "inactive", wantErrKey: "isActive"},
		{name: "null description is rejected", field: "description", value: nil, wantErrKey: "description"},
		{name: "object description is rejected", field: "description", value: map[string]any{"text": "half off"}, wantErrKey: "description"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Discount{
				Name:         "Update Offer " + tc.name,
				DiscountType: "offer",
				ValueType:    "percentage",
				Value:        10,
			}
			if err := d.Create(); err != nil {
				t.Fatalf("create: %v", err)
			}
			err := d.Update(map[string]any{tc.field: tc.value})
			if tc.wantErrKey != "" {
				if err == nil {
					t.Fatalf("expected error for %s %v, got nil", tc.field, tc.value)
				}
				var valErr validation.Errors
				if !errors.As(err, &valErr) {
					t.Fatalf("expected validation.Errors, got %v", err)
				}
				if _, ok := valErr[tc.wantErrKey]; !ok {
					t.Errorf("expected error key %q, got %v", tc.wantErrKey, valErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("update failed: %v", err)
			}
			var stored Discount
			if err := stored.GetByID(d.ID); err != nil {
				t.Fatalf("get: %v", err)
			}
			if stored.IsActive == nil || *stored.IsActive != tc.wantStored {
				t.Errorf("stored isActive = %v, want %d", stored.IsActive, tc.wantStored)
			}
		})
	}
}

func TestDiscount_Active(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	tests := []struct {
		name     string
		isActive *int
		want     bool
	}{
		{name: "nil is not active", isActive: nil, want: false},
		{name: "0 is not active", isActive: intPtr(0), want: false},
		{name: "1 is active", isActive: intPtr(1), want: true},
		{name: "2 is not active", isActive: intPtr(2), want: false},
		{name: "-1 is not active", isActive: intPtr(-1), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Discount{IsActive: tc.isActive}
			if got := d.Active(); got != tc.want {
				t.Errorf("Active() = %v, want %v", got, tc.want)
			}
		})
	}
}
