package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"

	"github.com/labstack/echo/v4"
)

func TestServerInfoRouteIncludesBuildMode(t *testing.T) {
	// Test values instead of the embedded secrets and peer settings.
	cfg := config.Load()
	cfg.PeerURL = ""
	cfg.SyncSecret = "test-sync-secret"
	cfg.PublishSecret = "test-publish-secret"
	cfg.PublicURL = "http://127.0.0.1:8080"
	cfg.JWTSecret = "test-jwt-secret"

	e := echo.New()
	SetupServerInfoRoutes(e.Group("/api"))

	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/server-info", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var envelope struct {
		Data struct {
			URL     string `json:"url"`
			Host    string `json:"host"`
			Port    string `json:"port"`
			IsCloud bool   `json:"isCloud"`
		}
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.URL == "" || envelope.Data.Host == "" || envelope.Data.Port == "" {
		t.Fatalf("incomplete server info: %+v", envelope.Data)
	}
	if envelope.Data.IsCloud != buildmode.Cloud {
		t.Fatalf("isCloud = %v, want %v", envelope.Data.IsCloud, buildmode.Cloud)
	}

	legacy := httptest.NewRecorder()
	e.ServeHTTP(legacy, httptest.NewRequest(http.MethodGet, "/api/server-url", nil))
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("legacy route status = %d, want 404", legacy.Code)
	}
}
