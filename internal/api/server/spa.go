package server

import (
	"bytes"
	"io/fs"
	"net/http"
	"regexp"
	"strings"

	"clinic-api/client"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

var clinicTzMetaRegex = regexp.MustCompile(`(?i)<meta\s+name=["']clinic-timezone["']\s+content=["'][^"']*["']\s*/?>`)

// injectClinicTimezone inserts or updates the clinic-timezone meta tag in rawHTML.
func injectClinicTimezone(rawHTML []byte, tz string) []byte {
	if tz == "" {
		tz = "Asia/Beirut"
	}
	metaTag := []byte(`<meta name="clinic-timezone" content="` + tz + `">`)
	if clinicTzMetaRegex.Match(rawHTML) {
		return clinicTzMetaRegex.ReplaceAllLiteral(rawHTML, metaTag)
	}
	lower := bytes.ToLower(rawHTML)
	headIdx := bytes.Index(lower, []byte("<head"))
	if headIdx != -1 {
		closeIdx := bytes.IndexByte(rawHTML[headIdx:], '>')
		if closeIdx != -1 {
			insertAt := headIdx + closeIdx + 1
			res := make([]byte, 0, len(rawHTML)+len(metaTag)+1)
			res = append(res, rawHTML[:insertAt]...)
			res = append(res, metaTag...)
			res = append(res, rawHTML[insertAt:]...)
			return res
		}
	}
	return rawHTML
}

func spaHandler() echo.HandlerFunc {
	spaFS := client.DistFS()
	indexHTML, err := fs.ReadFile(spaFS, "index.html")
	if err != nil {
		indexHTML = []byte(`<!doctype html><meta charset="utf-8"><title>Zeal Clinic</title><body>Frontend not built.</body>`)
	}
	fileServer := http.FileServer(http.FS(spaFS))

	return func(c echo.Context) error {
		req := c.Request()
		path := req.URL.Path
		if path == "/api" || strings.HasPrefix(path, "/api/") || path == "/health" {
			return echo.ErrNotFound
		}
		rel := strings.TrimPrefix(path, "/")
		if rel != "" {
			if f, err := spaFS.Open(rel); err == nil {
				stat, statErr := f.Stat()
				f.Close()
				if statErr == nil && !stat.IsDir() {
					fileServer.ServeHTTP(c.Response(), req)
					return nil
				}
			}
		}
		page := injectClinicTimezone(indexHTML, store.ClinicTimezoneName())
		return c.HTMLBlob(http.StatusOK, page)
	}
}
