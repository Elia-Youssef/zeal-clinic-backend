package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/config"

	"github.com/labstack/echo/v4"
)

func TestServerInfoReportsTheLoopbackItListensOn(t *testing.T) {
	cfg := config.Load()
	cfg.PublicURL = "" // a cloud's public address would answer instead

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	e := echo.New()
	e.Listener = listener

	recorder := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/server-info", nil), recorder)
	if err := GetServerInfo(c); err != nil {
		t.Fatal(err)
	}
	var envelope struct{ Data ServerInfo }
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	want := ServerInfo{URL: "http://127.0.0.1:" + cfg.Port, Host: "127.0.0.1", Port: cfg.Port, IsCloud: buildmode.Cloud}
	if envelope.Data != want {
		t.Fatalf("server info = %+v, want %+v", envelope.Data, want)
	}
}
