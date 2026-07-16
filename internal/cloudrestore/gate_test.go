package cloudrestore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestMaintenanceGateDrainsAndRejectsNewAPIRequests(t *testing.T) {
	gate := newMaintenanceGate()
	gate.mu.Lock()
	gate.active = 2 // restore request plus one finite in-flight request
	gate.mu.Unlock()

	done := make(chan error, 1)
	go func() { done <- gate.begin() }()
	select {
	case err := <-done:
		t.Fatalf("begin returned before other request drained: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	gate.mu.Lock()
	gate.active--
	gate.cond.Broadcast()
	gate.mu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("begin did not finish after request drained")
	}

	e := echo.New()
	e.Use(gate.middleware())
	e.GET("/api/test", func(c echo.Context) error { return c.NoContent(http.StatusNoContent) })
	e.GET("/files/test.pdf", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/health", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	apiRec := httptest.NewRecorder()
	e.ServeHTTP(apiRec, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	if apiRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("API status = %d, want 503", apiRec.Code)
	}
	fileRec := httptest.NewRecorder()
	e.ServeHTTP(fileRec, httptest.NewRequest(http.MethodGet, "/files/test.pdf", nil))
	if fileRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("file status = %d, want 503", fileRec.Code)
	}
	healthRec := httptest.NewRecorder()
	e.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthRec.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", healthRec.Code)
	}
	gate.end()
}

func TestMaintenanceGateKeepsReadyLongLivedRequestConnected(t *testing.T) {
	gate := newMaintenanceGate()
	e := echo.New()
	e.Use(gate.middleware())
	ready := make(chan struct{})
	stop := make(chan struct{})
	streamDone := make(chan struct{})
	e.GET("/api/events", func(c echo.Context) error {
		MarkLongLivedReady(c)
		close(ready)
		<-stop
		return c.NoContent(http.StatusOK)
	})
	e.POST("/api/cloud-restore", func(c echo.Context) error {
		if err := gate.begin(); err != nil {
			return err
		}
		gate.end()
		return c.NoContent(http.StatusOK)
	})

	go func() {
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/events", nil))
		close(streamDone)
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("long-lived request did not become ready")
	}

	restoreDone := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/cloud-restore", nil))
		restoreDone <- recorder.Code
	}()
	select {
	case code := <-restoreDone:
		if code != http.StatusOK {
			t.Fatalf("restore status = %d, want 200", code)
		}
	case <-time.After(time.Second):
		t.Fatal("ready long-lived request blocked maintenance")
	}
	select {
	case <-streamDone:
		t.Fatal("maintenance disconnected the long-lived request")
	default:
	}
	close(stop)
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("long-lived request did not stop")
	}
}
