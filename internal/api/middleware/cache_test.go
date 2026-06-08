package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"
)

// resetCache wipes the package-level cache between tests so they don't bleed.
func resetCache() {
	cacheMu.Lock()
	cache = make(map[string]map[string]*cachedResponse)
	cacheBytes = 0
	cacheMu.Unlock()
}

// counter handler returns a body containing an incrementing call count so we
// can detect whether the next handler was invoked. Atomic to allow concurrent
// use in the parallel-safety test.
type counter struct{ n atomic.Int64 }

func (c *counter) handler(echoCtx echo.Context) error {
	v := c.n.Add(1)
	return echoCtx.JSON(http.StatusOK, map[string]int64{"n": v})
}

// runReq runs a request through the cache middleware mounted on a fresh Echo
// instance. paramNames sets path param keys (only the count matters for the
// "skip if path params present" branch).
func runReq(method, target string, mw echo.MiddlewareFunc, h echo.HandlerFunc, paramNames []string) (*httptest.ResponseRecorder, echo.Context) {
	e := echo.New()
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if len(paramNames) > 0 {
		c.SetParamNames(paramNames...)
		vals := make([]string, len(paramNames))
		for i := range paramNames {
			vals[i] = "v"
		}
		c.SetParamValues(vals...)
	}
	_ = mw(h)(c)
	return rec, c
}

func TestCacheMiddleware_PanicsWithoutKeys(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic when no keys provided")
		}
	}()
	CacheMiddleware()
}

func TestCacheMiddleware_GET_CachesAndReturnsStored(t *testing.T) {
	resetCache()
	cnt := &counter{}
	mw := CacheMiddleware("things")

	// 1st call: cache miss, handler runs.
	rec1, _ := runReq(http.MethodGet, "/api/things", mw, cnt.handler, nil)
	if rec1.Code != http.StatusOK {
		t.Fatalf("rec1.Code = %d", rec1.Code)
	}
	if cnt.n.Load() != 1 {
		t.Errorf("handler should have run once, n=%d", cnt.n.Load())
	}
	body1 := rec1.Body.String()
	if !strings.Contains(body1, `"n":1`) {
		t.Errorf("body1 = %s", body1)
	}

	// 2nd call: cache hit, handler must NOT run again.
	rec2, _ := runReq(http.MethodGet, "/api/things", mw, cnt.handler, nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("rec2.Code = %d", rec2.Code)
	}
	if cnt.n.Load() != 1 {
		t.Errorf("handler must not run on cache hit, n=%d", cnt.n.Load())
	}
	if rec2.Body.String() != body1 {
		t.Errorf("cached body differs: %q vs %q", rec2.Body.String(), body1)
	}
}

func TestCacheMiddleware_DifferentURIsCachedSeparately(t *testing.T) {
	resetCache()
	cnt := &counter{}
	mw := CacheMiddleware("things")

	runReq(http.MethodGet, "/api/things?a=1", mw, cnt.handler, nil)
	runReq(http.MethodGet, "/api/things?a=2", mw, cnt.handler, nil)
	runReq(http.MethodGet, "/api/things?a=1", mw, cnt.handler, nil) // hit
	runReq(http.MethodGet, "/api/things?a=2", mw, cnt.handler, nil) // hit
	if cnt.n.Load() != 2 {
		t.Errorf("handler should run exactly 2 times (one per distinct URI), got %d", cnt.n.Load())
	}
}

func TestCacheMiddleware_FilterParamBypassesCache(t *testing.T) {
	resetCache()
	cnt := &counter{}
	mw := CacheMiddleware("things")

	runReq(http.MethodGet, "/api/things?filter=abc", mw, cnt.handler, nil)
	runReq(http.MethodGet, "/api/things?filter=abc", mw, cnt.handler, nil)
	if cnt.n.Load() != 2 {
		t.Errorf("filter URI must always run handler, got %d", cnt.n.Load())
	}
	// And the response must NOT have been cached.
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if entries, ok := cache["things"]; ok && len(entries) > 0 {
		t.Errorf("filter responses should not be cached, but cache has %d entries", len(entries))
	}
}

func TestCacheMiddleware_PathParamsBypassCache(t *testing.T) {
	resetCache()
	cnt := &counter{}
	mw := CacheMiddleware("things")

	runReq(http.MethodGet, "/api/things/abc", mw, cnt.handler, []string{"id"})
	runReq(http.MethodGet, "/api/things/abc", mw, cnt.handler, []string{"id"})
	if cnt.n.Load() != 2 {
		t.Errorf("per-resource (path param) requests must not be cached, got %d", cnt.n.Load())
	}
}

func TestCacheMiddleware_NonGETInvalidates(t *testing.T) {
	resetCache()
	cnt := &counter{}
	mw := CacheMiddleware("things", "analytics")

	// Seed the cache via two GETs on different keys.
	runReq(http.MethodGet, "/api/things", mw, cnt.handler, nil)
	runReq(http.MethodGet, "/api/things?x=1", mw, cnt.handler, nil)
	cacheMu.RLock()
	if len(cache["things"]) != 2 {
		t.Errorf("expected 2 cached entries, got %d", len(cache["things"]))
	}
	cacheMu.RUnlock()

	// Pre-seed analytics directly.
	cacheMu.Lock()
	cache["analytics"] = map[string]*cachedResponse{"/api/analytics": {body: []byte("{}")}}
	cacheMu.Unlock()

	// POST should invalidate every key listed.
	runReq(http.MethodPost, "/api/things", mw, cnt.handler, nil)

	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if _, ok := cache["things"]; ok {
		t.Errorf("things cache should be wiped after POST")
	}
	if _, ok := cache["analytics"]; ok {
		t.Errorf("analytics cache (secondary key) should be wiped after POST")
	}
}

func TestCacheMiddleware_PUT_DELETE_AlsoInvalidate(t *testing.T) {
	for _, m := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		t.Run(m, func(t *testing.T) {
			resetCache()
			cnt := &counter{}
			mw := CacheMiddleware("things")
			runReq(http.MethodGet, "/api/things", mw, cnt.handler, nil)
			cacheMu.RLock()
			seeded := len(cache["things"]) == 1
			cacheMu.RUnlock()
			if !seeded {
				t.Fatalf("seed failed for %s", m)
			}
			runReq(m, "/api/things", mw, cnt.handler, nil)
			cacheMu.RLock()
			defer cacheMu.RUnlock()
			if _, ok := cache["things"]; ok {
				t.Errorf("%s did not invalidate cache", m)
			}
		})
	}
}

func TestCacheMiddleware_NonSuccessNotCached(t *testing.T) {
	resetCache()
	mw := CacheMiddleware("things")
	calls := 0
	h := func(c echo.Context) error {
		calls++
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "boom"})
	}
	runReq(http.MethodGet, "/api/things", mw, h, nil)
	cacheMu.RLock()
	cached := len(cache["things"])
	cacheMu.RUnlock()
	if cached != 0 {
		t.Errorf("5xx must not be cached, got %d entries", cached)
	}
	// Second call still hits handler.
	runReq(http.MethodGet, "/api/things", mw, h, nil)
	if calls != 2 {
		t.Errorf("handler should run twice, got %d", calls)
	}
}

func TestCacheMiddleware_4xxNotCached(t *testing.T) {
	resetCache()
	mw := CacheMiddleware("things")
	h := func(c echo.Context) error { return c.JSON(http.StatusBadRequest, map[string]string{"error": "x"}) }
	runReq(http.MethodGet, "/api/things", mw, h, nil)
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if len(cache["things"]) != 0 {
		t.Errorf("4xx must not be cached")
	}
}

func TestCacheMiddleware_3xxIsCached(t *testing.T) {
	// The middleware caches everything 200..399, by design.
	resetCache()
	mw := CacheMiddleware("things")
	h := func(c echo.Context) error {
		return c.JSON(http.StatusMovedPermanently, map[string]string{"x": "y"})
	}
	runReq(http.MethodGet, "/api/things", mw, h, nil)
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if len(cache["things"]) != 1 {
		t.Errorf("3xx response should be cached, got %d", len(cache["things"]))
	}
}

func TestInvalidateCache_NonExistentKey(t *testing.T) {
	resetCache()
	// Should not panic for unknown key.
	InvalidateCache("does-not-exist")
}

func TestInvalidateCacheAll_Empties(t *testing.T) {
	resetCache()
	cacheMu.Lock()
	cache["a"] = map[string]*cachedResponse{"/x": {body: []byte("xx")}}
	cache["b"] = map[string]*cachedResponse{"/y": {body: []byte("yy")}}
	cacheMu.Unlock()
	InvalidateCacheAll()
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if len(cache) != 0 {
		t.Errorf("InvalidateCacheAll left %d keys", len(cache))
	}
}

func TestCacheSize_FormatsBytesKBMB(t *testing.T) {
	resetCache()
	cacheMu.Lock()
	cacheBytes = 100
	cacheMu.Unlock()
	if b, label := CacheSize(); b != 100 || !strings.HasSuffix(label, " B") {
		t.Errorf("100B: got %d %q", b, label)
	}

	cacheMu.Lock()
	cacheBytes = 2100
	cacheMu.Unlock()
	if b, label := CacheSize(); b != 2100 || !strings.HasSuffix(label, " KB") {
		t.Errorf("KB tier: got %d %q", b, label)
	}

	cacheMu.Lock()
	cacheBytes = 2 * 1024 * 1024
	cacheMu.Unlock()
	if _, label := CacheSize(); !strings.HasSuffix(label, " MB") {
		t.Errorf("MB tier: got %q", label)
	}
}

func TestCacheMiddleware_AutoClearWhenOver30MB(t *testing.T) {
	resetCache()
	mw := CacheMiddleware("big")
	// Pre-fill cache with 30MB of data under a different key so the new
	// successful GET pushes it over the threshold.
	cacheMu.Lock()
	cache["other"] = map[string]*cachedResponse{"/x": {body: make([]byte, 30*1024*1024)}}
	cacheBytes = 30 * 1024 * 1024
	cacheMu.Unlock()

	h := func(c echo.Context) error {
		// A small successful body; what matters is total cache size.
		return c.JSON(http.StatusOK, map[string]string{"ok": "yes"})
	}
	runReq(http.MethodGet, "/api/big", mw, h, nil)

	cacheMu.RLock()
	defer cacheMu.RUnlock()
	if len(cache) != 0 {
		t.Errorf("expected cache to auto-clear after >30MB, got %d keys", len(cache))
	}
}

func TestCustomWriter_WritesToBothBufAndDownstream(t *testing.T) {
	resetCache()
	mw := CacheMiddleware("things")
	body := []byte(`{"hello":"world"}`)
	h := func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		c.Response().WriteHeader(http.StatusOK)
		_, err := c.Response().Writer.Write(body)
		return err
	}

	rec, _ := runReq(http.MethodGet, "/api/things", mw, h, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Body.String() != string(body) {
		t.Errorf("downstream got %q", rec.Body.String())
	}

	// Cached body must equal what the handler wrote.
	cacheMu.RLock()
	entry, ok := cache["things"]["/api/things"]
	cacheMu.RUnlock()
	if !ok {
		t.Fatalf("no cache entry created")
	}
	if string(entry.body) != string(body) {
		t.Errorf("cached body = %q want %q", entry.body, body)
	}
}

func TestCacheMiddleware_HitReturnsRawJSONBlob(t *testing.T) {
	resetCache()
	mw := CacheMiddleware("things")

	// Seed with a custom body.
	cacheMu.Lock()
	cache["things"] = map[string]*cachedResponse{
		"/api/things": {body: []byte(`{"cached":true}`)},
	}
	cacheMu.Unlock()

	// next must not be invoked.
	called := false
	h := func(c echo.Context) error {
		called = true
		return c.JSON(http.StatusOK, map[string]any{"cached": false})
	}
	rec, _ := runReq(http.MethodGet, "/api/things", mw, h, nil)
	if called {
		t.Errorf("handler must not run on hit")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"cached":true`) {
		t.Errorf("expected cached body, got %q", rec.Body.String())
	}
}

func TestCacheMiddleware_ConcurrentReadsAreSafe(t *testing.T) {
	resetCache()
	mw := CacheMiddleware("things")
	cnt := &counter{}
	// Seed once.
	runReq(http.MethodGet, "/api/things", mw, cnt.handler, nil)

	const goroutines = 50
	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			// Mix hits on /api/things (cached) and writes on different URIs.
			if i%2 == 0 {
				runReq(http.MethodGet, "/api/things", mw, cnt.handler, nil)
			} else {
				runReq(http.MethodGet, fmt.Sprintf("/api/things?x=%d", i), mw, cnt.handler, nil)
			}
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
