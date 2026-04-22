package middleware

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/labstack/echo/v4"
)

type cachedResponse struct {
	body []byte
}

var (
	cache   = make(map[string]map[string]*cachedResponse) // key -> uri -> response
	cacheMu sync.RWMutex
)

func CacheMiddleware(keys ...string) echo.MiddlewareFunc {
	if len(keys) == 0 {
		panic("CacheMiddleware requires at least one cache key")
	}
	primary := keys[0]

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Method != http.MethodGet {
				for _, k := range keys {
					InvalidateCache(k)
				}
				return next(c)
			}

			url := c.Request().URL.RequestURI()
			if strings.Contains(url, "filter") || len(c.ParamNames()) > 0 {
				return next(c)
			}

			cacheMu.RLock()
			entry, found := cache[primary][url]
			cacheMu.RUnlock()

			if found {
				return c.JSONBlob(http.StatusOK, entry.body)
			}

			buf := &bytes.Buffer{}
			c.Response().Writer = &CustomWriter{ResponseWriter: c.Response().Writer, buf: buf}

			if err := next(c); err != nil {
				return err
			}

			if c.Response().Status >= 200 && c.Response().Status < 400 {
				cacheMu.Lock()
				if cache[primary] == nil {
					cache[primary] = make(map[string]*cachedResponse)
				}
				cache[primary][url] = &cachedResponse{body: buf.Bytes()}
				cacheMu.Unlock()
				size, sizeString := CacheSize()
				log.Println("Updated cache size:", sizeString)
				if size > 30*1024*1024 {
					log.Println("Cache size exceeded 30MB, clearing cache")
					InvalidateCacheAll()
				}
			}

			return nil
		}
	}
}

type CustomWriter struct {
	http.ResponseWriter
	buf *bytes.Buffer
}

func (c *CustomWriter) Write(b []byte) (int, error) {
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

func InvalidateCache(key string) {
	cacheMu.Lock()
	_, existed := cache[key]
	delete(cache, key)
	cacheMu.Unlock()
	if existed {
		log.Println("Invalidated cache:", key)
	}
}

func InvalidateCacheAll() {
	cacheMu.Lock()
	cache = make(map[string]map[string]*cachedResponse)
	cacheMu.Unlock()
}

func CacheSize() (int, string) {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	bytes := 0
	for _, group := range cache {
		for _, entry := range group {
			bytes += len(entry.body)
		}
	}
	var label string
	switch {
	case bytes >= 1024*1024:
		label = fmt.Sprintf("%.2f MB", float64(bytes)/1024/1024)
	case bytes >= 1024:
		label = fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	default:
		label = fmt.Sprintf("%d B", bytes)
	}
	return bytes, label
}
