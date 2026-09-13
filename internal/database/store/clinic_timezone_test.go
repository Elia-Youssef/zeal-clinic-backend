package store

import "testing"

// TestSetClinicTimezone checks that a valid zone replaces the cached location
// and its name, and that an unknown zone is refused and leaves both alone.
func TestSetClinicTimezone(t *testing.T) {
	orig := ClinicTimezoneName()
	t.Cleanup(func() {
		if err := SetClinicTimezone(orig); err != nil {
			t.Errorf("restore %q: %v", orig, err)
		}
	})

	if err := SetClinicTimezone("Europe/London"); err != nil {
		t.Fatalf("a valid zone: %v", err)
	}
	if got := ClinicTimezoneName(); got != "Europe/London" {
		t.Errorf("ClinicTimezoneName() = %q, want Europe/London", got)
	}
	if got := ClinicLocation().String(); got != "Europe/London" {
		t.Errorf("ClinicLocation() = %q, want Europe/London", got)
	}

	if err := SetClinicTimezone("Invalid/Zone"); err == nil {
		t.Fatal("an unknown zone was accepted")
	}
	if got := ClinicTimezoneName(); got != "Europe/London" {
		t.Errorf("after an unknown zone ClinicTimezoneName() = %q, want Europe/London unchanged", got)
	}
	if got := ClinicLocation().String(); got != "Europe/London" {
		t.Errorf("after an unknown zone ClinicLocation() = %q, want Europe/London unchanged", got)
	}
}
