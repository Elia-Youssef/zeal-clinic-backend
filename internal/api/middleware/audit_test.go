package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestCreatedID(t *testing.T) {
	cases := map[string]string{
		`{"Data":{"id":"p-1","firstName":"X"},"Error":"","Success":true}`: "p-1",
		`{"Data":{"id":"EUR"},"Success":true}`:                            "EUR",
		`{"Data":[{"id":"p-1"}],"Success":true}`:                          "",
		`{"Data":null,"Success":true}`:                                    "",
		`{"Data":{"id":"p-1","firstName":"X`:                              "",
		`not json`:                                                        "",
	}
	for answer, want := range cases {
		if got := createdID([]byte(answer)); got != want {
			t.Errorf("createdID(%q) = %q want %q", answer, got, want)
		}
	}
}

func TestAnswerCapture(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &answerCapture{ResponseWriter: rec}

	// A write that stops ten bytes short of the cap, then one that crosses
	// it: both pass through unchanged, whatever the capture keeps of them.
	head := strings.Repeat("a", createdAnswerLimit-10)
	if n, err := w.Write([]byte(head)); err != nil || n != len(head) {
		t.Fatalf("Write = %d, %v want %d, <nil>", n, err, len(head))
	}
	tail := strings.Repeat("b", 1024)
	if n, err := w.Write([]byte(tail)); err != nil || n != len(tail) {
		t.Fatalf("Write = %d, %v want %d, <nil>", n, err, len(tail))
	}
	if got := rec.Body.String(); got != head+tail {
		t.Error("the answer did not pass through unchanged")
	}
	// The capture fills to the cap with the first write and keeps only the
	// first ten bytes of the one that crosses it.
	if w.body.Len() != createdAnswerLimit {
		t.Errorf("kept %d bytes, want the %d-byte cap", w.body.Len(), createdAnswerLimit)
	}
	if ending := strings.Repeat("a", 10) + strings.Repeat("b", 10); !strings.HasSuffix(w.body.String(), ending) {
		t.Error("the cap kept bytes from past the limit")
	}
}

func TestMapMethodToAction(t *testing.T) {
	cases := map[string]string{
		"POST":   "create",
		"PUT":    "update",
		"DELETE": "delete",
		"GET":    "unknown",
		"PATCH":  "update",
		"weird":  "unknown",
		"":       "unknown",
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
