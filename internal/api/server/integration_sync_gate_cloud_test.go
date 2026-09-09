//go:build cloud

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"clinic-api/internal/buildmode"
	syncpkg "clinic-api/internal/sync"
)

// openCriticalSyncGate opens the cloud's critical-sync gate through the same
// handshake a clinic uses: it holds /api/sync/events open, reads the session
// token from the hello event and confirms it with /api/sync/ready. The gate
// closes again when the returned func or the test cleanup ends the stream.
func openCriticalSyncGate(t *testing.T) (closeGate func()) {
	t.Helper()
	srv := httptest.NewServer(CreateServer())
	ctx, cancel := context.WithCancel(context.Background())
	tokens := make(chan string, 1)
	failed := make(chan error, 1)
	done := make(chan struct{})

	var once sync.Once
	closeGate = func() {
		once.Do(func() {
			cancel()
			<-done
			srv.Close()
		})
	}
	t.Cleanup(closeGate)

	go func() {
		defer close(done)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/sync/events", nil)
		if err != nil {
			failed <- err
			return
		}
		req.Header.Set("Accept", "text/event-stream")
		setSyncHeaders(req)
		resp, err := srv.Client().Do(req)
		if err != nil {
			failed <- err
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			failed <- fmt.Errorf("status %d", resp.StatusCode)
			return
		}
		r := bufio.NewReader(resp.Body)
		token, err := readHelloToken(r)
		if err != nil {
			failed <- err
			return
		}
		tokens <- token
		// Keep the stream open until the gate is closed.
		_, _ = io.Copy(io.Discard, r)
	}()

	var token string
	select {
	case token = <-tokens:
	case err := <-failed:
		t.Fatalf("sync events: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("sync events: no hello event")
	}

	body, err := json.Marshal(map[string]string{"session_token": token})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/sync/ready", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	setSyncHeaders(req)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("sync ready: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync ready: status %d", resp.StatusCode)
	}
	if !syncpkg.CriticalWritesReady() {
		t.Fatal("critical-sync gate still closed after the handshake")
	}
	return closeGate
}

func setSyncHeaders(req *http.Request) {
	req.Header.Set("X-Sync-Secret", testSyncSecret)
	req.Header.Set("X-Sync-Version", buildmode.Version)
}

// readHelloToken reads server-sent events up to the hello event and returns
// its session token.
func readHelloToken(r *bufio.Reader) (string, error) {
	event := ""
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("read hello: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:") && event == "hello":
			var hello struct {
				SessionToken string `json:"session_token"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &hello); err != nil {
				return "", fmt.Errorf("decode hello: %w", err)
			}
			if hello.SessionToken == "" {
				return "", errors.New("hello without a session token")
			}
			return hello.SessionToken, nil
		}
	}
}

// On the cloud, the financial write routes stay closed until a clinic has
// connected and confirmed a sync cycle, and close again when it disconnects.
func TestCriticalSync_FinancialWritesWaitForSyncedClinic(t *testing.T) {
	setupTestEnv(t)
	e := CreateServer() // no clinic connected yet
	tok := adminToken(t, e)

	post := func() *httptest.ResponseRecorder {
		t.Helper()
		return doRequest(t, e, http.MethodPost, "/api/client-payments", asJSON(t, map[string]any{}), tok)
	}

	rec := post()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("before the handshake: status = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "sync_not_ready")

	closeGate := openCriticalSyncGate(t)
	// Past the gate, the empty body fails validation.
	if rec := post(); rec.Code != http.StatusBadRequest {
		t.Fatalf("after the handshake: status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}

	closeGate()
	rec = post()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("after the clinic disconnected: status = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
	containsString(t, rec.Body.String(), "sync_not_ready")
}
