package middleware

import (
	"clinic-api/internal/buildmode"

	"github.com/labstack/echo/v4"
)

// ClientIPExtractor tells echo where a request's client address comes from;
// the login rate limit, the audit log and the sync log read it through
// c.RealIP(). The clinic build is reached directly on the LAN, so it takes the
// peer address and ignores forwarding headers, which any client could set. The
// cloud build may run behind a reverse proxy on the same host or network, so
// it reads X-Forwarded-For, but only when the peer is a loopback or private
// address.
func ClientIPExtractor() echo.IPExtractor {
	if buildmode.Cloud {
		return ProxiedClientIP()
	}
	return DirectClientIP()
}

// DirectClientIP is the peer address of the connection.
func DirectClientIP() echo.IPExtractor {
	return echo.ExtractIPDirect()
}

// ProxiedClientIP reads X-Forwarded-For, trusting loopback and private
// network hops as proxies and nothing else.
func ProxiedClientIP() echo.IPExtractor {
	return echo.ExtractIPFromXFFHeader(echo.TrustLoopback(true), echo.TrustLinkLocal(false), echo.TrustPrivateNet(true))
}
