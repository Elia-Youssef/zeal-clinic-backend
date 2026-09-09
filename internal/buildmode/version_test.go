package buildmode

import "testing"

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{
		"dev":           false, // unstamped
		"1.0.3-dev":     false, // make build, make dev
		"1.0.3":         true,
		"0.0.0-systest": true,
		"":              true,
		"1.0.3-dev.1":   true,
		"1.0.3-develop": true,
		"1.0.3-rc1-dev": false,
		"development":   true,
	} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
	if Release() != IsRelease(Version) {
		t.Error("Release() disagrees with IsRelease(Version)")
	}
}
