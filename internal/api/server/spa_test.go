package server

import (
	"bytes"
	"testing"
)

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
