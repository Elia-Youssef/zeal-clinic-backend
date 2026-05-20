package middleware

import "testing"

func TestParseEntityFromPath(t *testing.T) {
	cases := []struct {
		path        string
		wantType    string
		wantID      string
		description string
	}{
		{"/api/patients", "patients", "", "top-level collection"},
		{"/api/patients/abc-123", "patients", "abc-123", "top-level resource"},
		// Nested collection: entityID stays as the parent ID (e.g. patient_id).
		{"/api/patients/abc-123/allergies", "patients/allergies", "abc-123", "nested collection (parent id kept)"},
		{"/api/patients/abc-123/allergies/xyz", "patients/allergies", "xyz", "nested resource"},
		{"/api/", "", "", "trailing slash"},
		{"/api/users/u1/actions", "users/actions", "u1", "actions sub-path (parent id kept)"},
	}
	for _, c := range cases {
		t.Run(c.description, func(t *testing.T) {
			gotT, gotID := parseEntityFromPath(c.path)
			if gotT != c.wantType {
				t.Errorf("type = %q want %q", gotT, c.wantType)
			}
			if gotID != c.wantID {
				t.Errorf("id = %q want %q", gotID, c.wantID)
			}
		})
	}
}

func TestMapMethodToAction(t *testing.T) {
	cases := map[string]string{
		"POST":    "create",
		"PUT":     "update",
		"DELETE":  "delete",
		"GET":     "unknown",
		"PATCH":   "unknown",
		"weird":   "unknown",
		"":        "unknown",
	}
	for in, want := range cases {
		if got := mapMethodToAction(in); got != want {
			t.Errorf("mapMethodToAction(%q) = %q want %q", in, got, want)
		}
	}
}

func TestPasswordRedactor(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"username":"x","password":"hunter2"}`, `{"username":"x","password":"[REDACTED]"}`},
		{`{"password" : "spaces"}`, `{"password" : "[REDACTED]"}`},
		{`{"password":""}`, `{"password":"[REDACTED]"}`},
		{`{"name":"x"}`, `{"name":"x"}`},
		{`{"password":"a","password":"b"}`, `{"password":"[REDACTED]","password":"[REDACTED]"}`},
	}
	for _, c := range cases {
		got := passwordRedactor.ReplaceAllString(c.in, `$1"[REDACTED]"`)
		if got != c.want {
			t.Errorf("redact(%q) = %q want %q", c.in, got, c.want)
		}
	}
}
