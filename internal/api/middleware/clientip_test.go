package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"clinic-api/internal/buildmode"

	"github.com/labstack/echo/v4"
)

func forwardedRequest(remoteAddr, forwardedFor string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		req.Header.Set(echo.HeaderXForwardedFor, forwardedFor)
	}
	return req
}

func TestDirectClientIPIgnoresForwardingHeaders(t *testing.T) {
	extract := DirectClientIP()
	for _, tc := range []struct{ remote, forwarded, want string }{
		{"203.0.113.9:4321", "", "203.0.113.9"},
		{"203.0.113.9:4321", "198.51.100.7", "203.0.113.9"},
		{"127.0.0.1:4321", "198.51.100.7", "127.0.0.1"},
		{"[::1]:4321", "198.51.100.7", "::1"},
	} {
		if got := extract(forwardedRequest(tc.remote, tc.forwarded)); got != tc.want {
			t.Errorf("direct: peer %s, forwarded %q: got %q, want %q", tc.remote, tc.forwarded, got, tc.want)
		}
	}
}

func TestProxiedClientIPTrustsOnlyLocalProxies(t *testing.T) {
	extract := ProxiedClientIP()
	for _, tc := range []struct{ remote, forwarded, want string }{
		{"127.0.0.1:4321", "198.51.100.7", "198.51.100.7"},           // a proxy on the same host
		{"10.1.2.3:4321", "198.51.100.7", "198.51.100.7"},            // a proxy on the private network
		{"127.0.0.1:4321", "198.51.100.7, 10.0.0.8", "198.51.100.7"}, // two trusted hops
		{"203.0.113.9:4321", "198.51.100.7", "203.0.113.9"},          // a public peer forwards nothing
		{"169.254.10.10:4321", "198.51.100.7", "169.254.10.10"},      // link-local is not a proxy
		{"127.0.0.1:4321", "", "127.0.0.1"},
	} {
		if got := extract(forwardedRequest(tc.remote, tc.forwarded)); got != tc.want {
			t.Errorf("proxied: peer %s, forwarded %q: got %q, want %q", tc.remote, tc.forwarded, got, tc.want)
		}
	}
}

// A forwarded address sent through a loopback peer is ignored by the clinic
// build and honored by the cloud build.
func TestClientIPExtractorPerBuild(t *testing.T) {
	got := ClientIPExtractor()(forwardedRequest("127.0.0.1:4321", "198.51.100.7"))
	want := "127.0.0.1"
	if buildmode.Cloud {
		want = "198.51.100.7"
	}
	if got != want {
		t.Errorf("cloud build %v: got %q, want %q", buildmode.Cloud, got, want)
	}
}
