package server

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestLooksLikeAsset(t *testing.T) {
	for rel, want := range map[string]bool{
		"assets/index-abc123.js":   true,
		"assets/":                  true,
		"icon.ico":                 true,
		"fonts/inter.woff2":        true,
		"patients/some-id/x.png":   true,
		"":                         false,
		"patients":                 false,
		"patients/some-id/details": false,
		"settings/v1.2/general":    false,
	} {
		if got := looksLikeAsset(rel); got != want {
			t.Errorf("looksLikeAsset(%q) = %v, want %v", rel, got, want)
		}
	}
}

// A missing asset answers 404 while the root and the app routes keep the shell.
func TestSPAHandlerAnswers404ForMissingAssets(t *testing.T) {
	setupTestEnv(t)
	e := newTestServer(t)
	for reqPath, want := range map[string]int{
		"/assets/index-stale.js":    http.StatusNotFound,
		"/assets/":                  http.StatusNotFound,
		"/missing.png":              http.StatusNotFound,
		"/":                         http.StatusOK,
		"/settings":                 http.StatusOK,
		"/patients/some-id/details": http.StatusOK,
	} {
		rec := doRequest(t, e, http.MethodGet, reqPath, nil, "")
		if rec.Code != want {
			t.Errorf("GET %s: %d, want %d", reqPath, rec.Code, want)
		}
		if want == http.StatusOK && !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET %s: Content-Type %q, want the app shell", reqPath, rec.Header().Get("Content-Type"))
		}
	}
}

func TestInjectClinicTimezone(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		tz       string
		contains string
	}{
		{
			name:     "inject into head",
			input:    "<!doctype html><html><head><title>Zeal</title></head><body></body></html>",
			tz:       "Asia/Beirut",
			contains: `<meta name="clinic-timezone" content="Asia/Beirut">`,
		},
		{
			name:     "replace existing meta tag",
			input:    `<!doctype html><html><head><meta name="clinic-timezone" content="UTC"></head></html>`,
			tz:       "Asia/Beirut",
			contains: `<meta name="clinic-timezone" content="Asia/Beirut">`,
		},
		{
			name:     "custom timezone",
			input:    "<html><HEAD><title>Zeal</title></HEAD></html>",
			tz:       "Europe/Paris",
			contains: `<meta name="clinic-timezone" content="Europe/Paris">`,
		},
		{
			name:     "fallback to Asia/Beirut on empty tz",
			input:    "<html><head></head></html>",
			tz:       "",
			contains: `<meta name="clinic-timezone" content="Asia/Beirut">`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := injectClinicTimezone([]byte(tc.input), tc.tz)
			if !bytes.Contains(got, []byte(tc.contains)) {
				t.Errorf("injectClinicTimezone(%q, %q) = %s, want it to contain %s", tc.input, tc.tz, string(got), tc.contains)
			}
		})
	}
}
