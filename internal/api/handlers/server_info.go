package handlers

import (
	"net"
	"net/http"
	"net/url"

	"clinic-api/internal/api/httpx"
	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"

	"github.com/labstack/echo/v4"
)

type ServerURLInfo struct {
	URL  string `json:"url"`
	Host string `json:"host"`
	Port string `json:"port"`
}

func GetServerURL(c echo.Context) error {
	cfg := config.Current()

	if buildmode.Cloud && cfg.PublicURL != "" {
		host, port := splitURLHostPort(cfg.PublicURL, cfg.Port)
		return c.JSON(http.StatusOK, httpx.Response{
			Success: true,
			Data: ServerURLInfo{
				URL:  cfg.PublicURL,
				Host: host,
				Port: port,
			},
		})
	}

	port := cfg.Port
	host := localIPv4()

	return c.JSON(http.StatusOK, httpx.Response{
		Success: true,
		Data: ServerURLInfo{
			URL:  "http://" + net.JoinHostPort(host, port),
			Host: host,
			Port: port,
		},
	})
}

func splitURLHostPort(raw, fallbackPort string) (host, port string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw, fallbackPort
	}
	host = u.Hostname()
	port = u.Port()
	if port == "" {
		switch u.Scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		default:
			port = fallbackPort
		}
	}
	return host, port
}

func localIPv4() string {
	if ip := defaultRouteIPv4(); ip != "" {
		return ip
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}

	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip := ipFromAddr(addr)
			if ip == nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsUnspecified() {
				continue
			}
			if ip4.IsPrivate() {
				return ip4.String()
			}
			if fallback == "" && !ip4.IsLinkLocalUnicast() {
				fallback = ip4.String()
			}
		}
	}

	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}

func defaultRouteIPv4() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}
	ip4 := addr.IP.To4()
	if ip4 == nil || ip4.IsLoopback() || ip4.IsUnspecified() {
		return ""
	}
	return ip4.String()
}

func ipFromAddr(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}
