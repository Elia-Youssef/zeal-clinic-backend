package server

import (
	"net/http"
	"strings"
	"time"

	"clinic-api/internal/buildmode"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Request and response hardening: the server's header and idle timeouts, a
// body limit for the JSON routes, the headers every answer carries, the
// content security policy of the app shell, the CORS setup only a Vite dev
// server needs, and where a request's client address comes from.

const (
	// bodyLimit caps the request bodies of the JSON routes. The sync push and
	// the cloud restore upload carry whole tables or a database snapshot and
	// are exempt on the cloud build, the only one that receives them; no
	// other route reads a large body (nothing uploads files).
	bodyLimit = "16M"

	// readHeaderTimeout bounds how long a connection may take to send its
	// request headers. Bodies and responses stay unbounded on purpose: the
	// event streams stay open for hours and a restore uploads a whole database.
	readHeaderTimeout = 10 * time.Second

	// idleTimeout closes a kept-alive connection that sends no further
	// request, so quiet connections don't hold a goroutine and a socket
	// forever. An active request, an event stream or an upload is not idle
	// and is unaffected.
	idleTimeout = 120 * time.Second

	// appShellCSP is the content security policy of the app shell. The built
	// dashboard loads scripts, styles and fonts from its own origin, sets a few
	// inline style attributes, draws images from data and blob URLs, opens
	// PDFs as blob documents that inherit this policy (object-src lets the
	// browser's PDF viewer show them), talks to the API and the event stream
	// on the same origin and is never framed.
	appShellCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; object-src 'self' blob:; font-src 'self' data:; connect-src 'self'; " +
		"frame-ancestors 'none'; base-uri 'self'"
)

// devOrigins are the addresses of the dashboard's Vite dev server, the only
// cross-origin callers: the built binary serves the dashboard itself.
var devOrigins = []string{"http://localhost:5173", "http://127.0.0.1:5173"}

// bodyLimitMiddleware exempts the upload routes by the route that matched,
// never by the request path: routes match the raw path, so an encoded
// spelling of an exempt path reaches another route and stays limited. The
// middleware runs after routing, so c.Path() is the matched route's pattern.
func bodyLimitMiddleware() echo.MiddlewareFunc {
	return middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		Skipper: func(c echo.Context) bool {
			route := c.Path()
			return buildmode.Cloud && (route == "/api/cloud-restore" || strings.HasPrefix(route, "/api/sync/"))
		},
		Limit: bodyLimit,
	})
}

// securityHeaders marks every response as not to be sniffed, framed, indexed
// or leaked through the referrer. There is no HSTS: TLS is the proxy's job.
func securityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "same-origin")
			h.Set("X-Robots-Tag", "noindex, nofollow")
			return next(c)
		}
	}
}

func devCORS() echo.MiddlewareFunc {
	return middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: devOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
		AllowHeaders: []string{echo.HeaderAuthorization, echo.HeaderContentType},
		MaxAge:       600,
	})
}

// clientIPExtractor tells echo where a request's client address comes from;
// the login rate limit, the audit log and the sync log read it through
// c.RealIP(). The clinic build is reached directly on the LAN, so it takes
// the peer address and ignores forwarding headers, which any client could
// set. The cloud build may run behind a reverse proxy on the same host or
// network, so it reads X-Forwarded-For, but only when the peer is a loopback
// or private address.
func clientIPExtractor() echo.IPExtractor {
	if buildmode.Cloud {
		return proxiedClientIP()
	}
	return directClientIP()
}

// directClientIP is the peer address of the connection.
func directClientIP() echo.IPExtractor {
	return echo.ExtractIPDirect()
}

// proxiedClientIP reads X-Forwarded-For, trusting loopback and private
// network hops as proxies and nothing else.
func proxiedClientIP() echo.IPExtractor {
	return echo.ExtractIPFromXFFHeader(echo.TrustLoopback(true), echo.TrustLinkLocal(false), echo.TrustPrivateNet(true))
}
