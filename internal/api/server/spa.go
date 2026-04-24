package server

import (
	"io/fs"
	"net/http"
	"strings"

	"clinic-api/client"

	"github.com/labstack/echo/v4"
)

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
		return c.HTMLBlob(http.StatusOK, indexHTML)
	}
}
