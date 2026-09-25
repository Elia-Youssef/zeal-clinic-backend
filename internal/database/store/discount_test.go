package store

import "testing"

func TestDiscount_Create_IsActive(t *testing.T) {
	setupTestDB(t)

	t.Run("absent defaults to active", func(t *testing.T) {
		d := Discount{Name: "Spring Sale", DiscountType: "offer", ValueType: "percentage", Value: 10}
		if err := d.Create(); err != nil {
			t.Fatalf("create: %v", err)
		}
		var stored Discount
		if err := stored.GetByID(d.ID); err != nil {
			t.Fatalf("get: %v", err)
		}
		if stored.IsActive == nil {
			t.Fatal("isActive unset, want 1")
		}
		if *stored.IsActive != 1 {
			t.Errorf("isActive = %d, want 1", *stored.IsActive)
		}
	})

	t.Run("explicit 0 stays inactive", func(t *testing.T) {
		inactive := 0
		d := Discount{Name: "Planned Sale", DiscountType: "offer", ValueType: "percentage", Value: 10, IsActive: &inactive}
		if err := d.Create(); err != nil {
			t.Fatalf("create: %v", err)
		}
		var stored Discount
		if err := stored.GetByID(d.ID); err != nil {
			t.Fatalf("get: %v", err)
		}
		if stored.IsActive == nil {
			t.Fatal("isActive unset, want 0")
		}
		if *stored.IsActive != 0 {
			t.Errorf("isActive = %d, want 0", *stored.IsActive)
		}
	})
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
