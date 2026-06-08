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
	cache      = make(map[string]map[string]*cachedResponse) // key -> uri -> response
	cacheBytes int // running total, avoids walking the map per write
	cacheMu    sync.RWMutex
)

func CacheMiddleware(keys ...string) echo.MiddlewareFunc {
	return cacheMiddleware(false, keys...)
}

// CacheMiddlewareForce caches GET responses keyed by full request URI even
// when the route has path params or a "filter" query. Use for endpoints
// whose URL variation is bounded (e.g. per-employee weekly schedule keyed by
// id + date). Non-GET requests still invalidate the listed keys.
func CacheMiddlewareForce(keys ...string) echo.MiddlewareFunc {
	return cacheMiddleware(true, keys...)
}

func cacheMiddleware(force bool, keys ...string) echo.MiddlewareFunc {
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
			if !force && (strings.Contains(url, "filter") || len(c.ParamNames()) > 0) {
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
				body := buf.Bytes()
				cacheMu.Lock()
				if cache[primary] == nil {
					cache[primary] = make(map[string]*cachedResponse)
				}
				if old, ok := cache[primary][url]; ok {
					cacheBytes -= len(old.body)
				}
				cache[primary][url] = &cachedResponse{body: body}
				cacheBytes += len(body)
				over := cacheBytes > 30*1024*1024
				cacheMu.Unlock()
				if over {
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
	group, existed := cache[key]
	if existed {
		for _, e := range group {
			cacheBytes -= len(e.body)
		}
		delete(cache, key)
	}
	cacheMu.Unlock()
	if existed {
		log.Println("Invalidated cache:", key)
	}
}

func InvalidateCacheAll() {
	cacheMu.Lock()
	cache = make(map[string]map[string]*cachedResponse)
	cacheBytes = 0
	cacheMu.Unlock()
}

func CacheSize() (int, string) {
	cacheMu.RLock()
	total := cacheBytes
	cacheMu.RUnlock()
	var label string
	switch {
	case total >= 1024*1024:
		label = fmt.Sprintf("%.2f MB", float64(total)/1024/1024)
	case total >= 1024:
		label = fmt.Sprintf("%.2f KB", float64(total)/1024)
	default:
		label = fmt.Sprintf("%d B", total)
	}
	return total, label
}
