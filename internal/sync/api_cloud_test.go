//go:build cloud

package sync_test

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"clinic-api/internal/buildmode"
	syncpkg "clinic-api/internal/sync"

	"github.com/labstack/echo/v4"
)

const machineSecret = "test-sync-secret"

type machineServer struct {
	db  *sql.DB
	srv *httptest.Server
}

// newMachineServer serves the sync API of a cloud node the way the server
// mounts it, plus one ordinary write route behind the sync middleware.
func newMachineServer(t *testing.T) *machineServer {
	t.Helper()
	db := newNode(t)
	e := echo.New()
	e.Use(syncpkg.Middleware())
	(&syncpkg.API{DB: db, Secret: machineSecret}).RegisterRoutes(e)
	e.POST("/api/rooms", func(c echo.Context) error { return c.NoContent(http.StatusCreated) })
	srv := httptest.NewServer(e)
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	syncpkg.MarkSyncRecovered()
	t.Cleanup(syncpkg.MarkSyncRecovered)
	return &machineServer{db: db, srv: srv}
}

// call sends one request; secret and version are sent as headers when not
// empty. It returns the status and the body.
func (m *machineServer) call(t *testing.T, method, path, body, secret, version string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, m.srv.URL+path, r)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		req.Header.Set("X-Sync-Secret", secret)
	}
	if version != "" {
		req.Header.Set("X-Sync-Version", version)
	}
	resp, err := m.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, strings.TrimSpace(string(raw))
}

// peerCall is a request from a peer on the same build with the right secret.
func (m *machineServer) peerCall(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	return m.call(t, method, path, body, machineSecret, buildmode.Version)
}

var machineRoutes = []struct{ method, path string }{
	{http.MethodGet, "/api/sync/pull"},
	{http.MethodPost, "/api/sync/push"},
	{http.MethodPost, "/api/sync/ready"},
	{http.MethodPost, "/api/sync/failed"},
	{http.MethodGet, "/api/sync/events"},
	{http.MethodGet, "/api/sync/status"},
}

func TestSyncAPI_SecretIsCheckedFirst(t *testing.T) {
	m := newMachineServer(t)
	for _, r := range machineRoutes {
		for _, c := range []struct{ name, secret, version string }{
			{"no secret", "", buildmode.Version},
			{"wrong secret", "not-the-secret", buildmode.Version},
			{"wrong secret and version", "not-the-secret", "0.0.0"},
		} {
			status, body := m.call(t, r.method, r.path, "", c.secret, c.version)
			if status != http.StatusUnauthorized || body != `{"error":"unauthorized"}` {
				t.Errorf("%s %s with %s = %d %s, want 401 unauthorized", r.method, r.path, c.name, status, body)
			}
		}
	}
}

func TestSyncAPI_RefusesAnotherVersionExceptOnStatus(t *testing.T) {
	m := newMachineServer(t)
	for _, r := range machineRoutes {
		for _, v := range []string{"", "0.0.0"} {
			status, body := m.call(t, r.method, r.path, "", machineSecret, v)
			if r.path == "/api/sync/status" {
				if status != http.StatusOK {
					t.Errorf("status with version %q = %d %s, want 200", v, status, body)
				}
				continue
			}
			want := `{"cloud":"` + buildmode.Version + `","error":"version mismatch","peer":"` + v + `"}`
			if status != http.StatusConflict || body != want {
				t.Errorf("%s %s with version %q = %d %s, want 409 %s", r.method, r.path, v, status, body, want)
			}
		}
	}
}

// The secret is read from the X-Sync-Secret header only: a sync_secret query
// parameter is ignored, whatever it carries.
func TestSyncAPI_IgnoresTheSecretInTheQuery(t *testing.T) {
	m := newMachineServer(t)
	for _, path := range []string{"/api/sync/pull", "/api/sync/status"} {
		status, body := m.call(t, http.MethodGet, path+"?sync_secret="+machineSecret, "", "", buildmode.Version)
		if status != http.StatusUnauthorized || body != `{"error":"unauthorized"}` {
			t.Errorf("%s with the query secret = %d %s, want 401 unauthorized", path, status, body)
		}
	}
	if status, body := m.call(t, http.MethodGet, "/api/sync/status?sync_secret=wrong", "", machineSecret, ""); status != http.StatusOK {
		t.Errorf("status with the header secret and a wrong query value = %d %s, want 200", status, body)
	}
}

func TestSyncAPI_StatusReportsSequences(t *testing.T) {
	m := newMachineServer(t)
	mustExec(t, m.db, `INSERT INTO rooms (id, name, type) VALUES ('rooms-9', 'Status Room', 'General')`)
	status, body := m.call(t, http.MethodGet, "/api/sync/status", "", machineSecret, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d %s", status, body)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"self_id":         "cloud",
		"max_log_seq":     float64(maxSeq(t, m.db)),
		"last_pushed_seq": float64(0),
		"last_pulled_seq": float64(0),
	}
	if len(got) != len(want) {
		t.Fatalf("status body = %s", body)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("status %s = %v, want %v", k, got[k], v)
		}
	}
	if n := count(t, m.db, `SELECT COUNT(*) FROM sync_state WHERE peer = 'peer'`); n != 1 {
		t.Errorf("sync_state rows for the peer = %d, want 1 (created by the status read)", n)
	}
}

// Pull returns at most limit rows; a missing, non-positive or larger than
// 2000 limit falls back to 500. A since value prunes the rows up to it.
func TestSyncAPI_PullLimitAndPrune(t *testing.T) {
	m := newMachineServer(t)
	mustExec(t, m.db, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 2100)
		INSERT INTO sync_log (table_name, row_id, op) SELECT 'rooms', 'gone-' || i, 'delete' FROM n`)
	total := count(t, m.db, `SELECT COUNT(*) FROM sync_log`)
	top := maxSeq(t, m.db)

	pull := func(query string) syncpkg.PullResponse {
		t.Helper()
		status, body := m.peerCall(t, http.MethodGet, "/api/sync/pull"+query, "")
		if status != http.StatusOK {
			t.Fatalf("pull%s = %d %s", query, status, body)
		}
		var pr syncpkg.PullResponse
		if err := json.Unmarshal([]byte(body), &pr); err != nil {
			t.Fatal(err)
		}
		return pr
	}
	for _, c := range []struct {
		query string
		want  int
	}{
		{"", 500},
		{"?limit=0", 500},
		{"?limit=-5", 500},
		{"?limit=abc", 500},
		{"?limit=10", 10},
		{"?limit=2000", 2000},
		{"?limit=2001", 500},
	} {
		pr := pull(c.query)
		if len(pr.Rows) != c.want {
			t.Errorf("pull%s returned %d rows, want %d", c.query, len(pr.Rows), c.want)
		}
		if pr.MaxSeq != top {
			t.Errorf("pull%s max_seq = %d, want %d", c.query, pr.MaxSeq, top)
		}
	}
	if n := count(t, m.db, `SELECT COUNT(*) FROM sync_log`); n != total {
		t.Fatalf("pulls without since removed rows: %d -> %d", total, n)
	}

	var first int64
	if err := m.db.QueryRow(`SELECT MIN(seq) FROM sync_log`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	since := first + 99
	pr := pull("?since=" + strconv.FormatInt(since, 10) + "&limit=5")
	if len(pr.Rows) != 5 || pr.Rows[0].Seq != since+1 || pr.Rows[4].Seq != since+5 {
		t.Fatalf("pull after %d = %+v", since, pr.Rows)
	}
	if n := count(t, m.db, `SELECT COUNT(*) FROM sync_log WHERE seq <= ?`, since); n != 0 {
		t.Errorf("%d rows up to the confirmed seq %d are still in the outbox", n, since)
	}
	if n := count(t, m.db, `SELECT COUNT(*) FROM sync_log`); n != total-100 {
		t.Errorf("outbox has %d rows after the prune, want %d", n, total-100)
	}
}

// Push applies with the cloud policy and reports no_delete refusals.
func TestSyncAPI_PushAppliesRows(t *testing.T) {
	m := newMachineServer(t)
	loadFixture(t, m.db)
	setMark(t, m.db, "patients", "patients-1", "local", "2099-01-01T00:00:00Z")

	clinic := newNode(t)
	loadFixture(t, clinic)
	since := maxSeq(t, clinic)
	setMark(t, clinic, "patients", "patients-1", "remote", "2026-02-01T00:00:00Z")
	mustExec(t, clinic, `DELETE FROM invoices WHERE id = 'invoices-2'`)
	batch := outgoing(t, clinic, since)
	body, _ := json.Marshal(syncpkg.PushRequest{Rows: batch})

	status, resp := m.peerCall(t, http.MethodPost, "/api/sync/push", string(body))
	if status != http.StatusOK {
		t.Fatalf("push = %d %s", status, resp)
	}
	var pr syncpkg.PushResponse
	if err := json.Unmarshal([]byte(resp), &pr); err != nil {
		t.Fatal(err)
	}
	if pr.AppliedSeq != lastSeq(batch) {
		t.Errorf("applied_seq = %d, want %d", pr.AppliedSeq, lastSeq(batch))
	}
	if len(pr.Conflicts) != 1 || pr.Conflicts[0].Resolution != "no_delete" || pr.Conflicts[0].RowID != "invoices-2" {
		t.Errorf("conflicts = %+v, want one no_delete for invoices-2", pr.Conflicts)
	}
	if got, _ := markOf(t, m.db, "patients", "patients-1"); got != "remote" {
		t.Errorf("patients-1 = %q, want the pushed row", got)
	}

	if status, resp := m.peerCall(t, http.MethodPost, "/api/sync/push", `{"rows":[]}`); status != http.StatusOK || resp != `{"applied_seq":0}` {
		t.Errorf("empty push = %d %s", status, resp)
	}
	if status, resp := m.peerCall(t, http.MethodPost, "/api/sync/push", `{"rows":`); status != http.StatusBadRequest || !strings.HasPrefix(resp, `{"error":"decode: `) {
		t.Errorf("malformed push = %d %s", status, resp)
	}
}

// A push that fails to apply closes the write gate and notifies every active
// user once per failure episode.
func TestSyncAPI_FailedPushClosesTheGateAndNotifies(t *testing.T) {
	m := newMachineServer(t)
	s := openEvents(t, m)
	token := s.hello(t)
	if status, body := m.peerCall(t, http.MethodPost, "/api/sync/ready", `{"session_token":"`+token+`"}`); status != http.StatusOK {
		t.Fatalf("ready = %d %s", status, body)
	}
	if !syncpkg.CriticalWritesReady() {
		t.Fatal("gate closed after ready")
	}

	bad := `{"rows":[{"seq":7,"table":"patients","row_id":"patients-9","op":"insert","row_json":{"first_name":"No"},"created_at":"2026-01-01T00:00:00Z"}]}`
	for i := 0; i < 2; i++ {
		status, body := m.peerCall(t, http.MethodPost, "/api/sync/push", bad)
		want := `{"error":"[sync] apply push batch (1 rows): apply patients/patients-9: row_json missing id"}`
		if status != http.StatusInternalServerError || body != want {
			t.Fatalf("failing push = %d %s, want 500 %s", status, body, want)
		}
	}
	if syncpkg.CriticalWritesReady() {
		t.Error("gate still open after a failed push")
	}
	active := count(t, m.db, `SELECT COUNT(*) FROM users WHERE is_active = 1`)
	if n := count(t, m.db, `SELECT COUNT(*) FROM notifications WHERE action = 'sync-failure' AND title = 'Data sync failed'`); n != active {
		t.Errorf("sync failure notifications = %d, want one per active user (%d)", n, active)
	}
}

// The event stream opens a session with hello; ready and failed only accept
// the current session token, and the session ends with the stream.
func TestSyncAPI_EventsSessionReadyAndFailed(t *testing.T) {
	m := newMachineServer(t)
	s := openEvents(t, m)
	token := s.hello(t)
	if len(token) != 32 {
		t.Fatalf("session token %q, want 32 hex characters", token)
	}
	if !syncpkg.IsCloudConnected() {
		t.Error("peer not reported as connected")
	}
	if syncpkg.CriticalWritesReady() {
		t.Fatal("gate open before ready")
	}

	stale := `{"session_token":"` + strings.Repeat("0", 32) + `"}`
	current := `{"session_token":"` + token + `"}`
	expect := func(path, body string, wantStatus int, wantBody string) {
		t.Helper()
		status, got := m.peerCall(t, http.MethodPost, path, body)
		if status != wantStatus || got != wantBody {
			t.Fatalf("POST %s %s = %d %s, want %d %s", path, body, status, got, wantStatus, wantBody)
		}
	}
	expect("/api/sync/ready", stale, http.StatusConflict, `{"error":"stale sync session"}`)
	expect("/api/sync/ready", `{}`, http.StatusConflict, `{"error":"stale sync session"}`)
	if status, _ := m.peerCall(t, http.MethodPost, "/api/sync/ready", `{`); status != http.StatusBadRequest {
		t.Errorf("malformed ready = %d, want 400", status)
	}
	expect("/api/sync/ready", current, http.StatusOK, `{"ready":true}`)
	if !syncpkg.CriticalWritesReady() {
		t.Fatal("gate closed after ready")
	}

	expect("/api/sync/failed", stale, http.StatusConflict, `{"error":"stale sync session"}`)
	if !syncpkg.CriticalWritesReady() {
		t.Fatal("a stale failure closed the gate")
	}
	expect("/api/sync/failed", current, http.StatusOK, `{"ready":false}`)
	expect("/api/sync/failed", current, http.StatusOK, `{"ready":false}`)
	if syncpkg.CriticalWritesReady() {
		t.Fatal("gate open after failed")
	}
	active := count(t, m.db, `SELECT COUNT(*) FROM users WHERE is_active = 1`)
	if n := count(t, m.db, `SELECT COUNT(*) FROM notifications WHERE action = 'sync-failure'`); n != active {
		t.Errorf("failure notifications = %d, want one per active user (%d)", n, active)
	}
	expect("/api/sync/ready", current, http.StatusOK, `{"ready":true}`)

	// An ordinary write wakes the peer after the one-second debounce.
	if status, _ := m.call(t, http.MethodPost, "/api/rooms", `{}`, "", ""); status != http.StatusCreated {
		t.Fatalf("write route = %d", status)
	}
	if line := s.next(t, 5*time.Second); line != "event: sync_pending" {
		t.Fatalf("stream line after a write = %q, want event: sync_pending", line)
	}

	s.close()
	waitFor(t, "the gate to close after the stream ended", func() bool {
		return !syncpkg.CriticalWritesReady() && !syncpkg.IsCloudConnected()
	})
	expect("/api/sync/ready", current, http.StatusConflict, `{"error":"stale sync session"}`)
}

// A new stream replaces the session: the older token can't open the gate and
// its disconnect doesn't close the newer session.
func TestSyncAPI_NewerStreamReplacesTheSession(t *testing.T) {
	m := newMachineServer(t)
	first := openEvents(t, m)
	oldToken := first.hello(t)
	second := openEvents(t, m)
	newToken := second.hello(t)

	if status, _ := m.peerCall(t, http.MethodPost, "/api/sync/ready", `{"session_token":"`+oldToken+`"}`); status != http.StatusConflict {
		t.Errorf("ready with the replaced token = %d, want 409", status)
	}
	if status, _ := m.peerCall(t, http.MethodPost, "/api/sync/ready", `{"session_token":"`+newToken+`"}`); status != http.StatusOK {
		t.Fatalf("ready with the current token = %d, want 200", status)
	}
	first.close()
	waitFor(t, "the replaced stream to end", func() bool { return syncpkg.PeerStreams() == 1 })
	if !syncpkg.CriticalWritesReady() {
		t.Error("closing the replaced stream closed the current session")
	}
	second.close()
	waitFor(t, "the gate to close after the current stream ended", func() bool { return !syncpkg.CriticalWritesReady() })
}

func TestSyncAPI_NotMountedWithoutSecret(t *testing.T) {
	e := echo.New()
	(&syncpkg.API{DB: newNode(t)}).RegisterRoutes(e)
	for _, r := range e.Routes() {
		if strings.HasPrefix(r.Path, "/api/sync") {
			t.Errorf("route %s %s mounted without a secret", r.Method, r.Path)
		}
	}
}

type eventStream struct {
	lines  chan string
	cancel context.CancelFunc
	body   io.Closer
}

func openEvents(t *testing.T, m *machineServer) *eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.srv.URL+"/api/sync/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Sync-Secret", machineSecret)
	req.Header.Set("X-Sync-Version", buildmode.Version)
	resp, err := m.srv.Client().Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" {
		cancel()
		resp.Body.Close()
		t.Fatalf("events = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	s := &eventStream{lines: make(chan string, 64), cancel: cancel, body: resp.Body}
	go func() {
		defer close(s.lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			s.lines <- sc.Text()
		}
	}()
	t.Cleanup(s.close)
	return s
}

// next returns the next non-empty line of the stream.
func (s *eventStream) next(t *testing.T, timeout time.Duration) string {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-s.lines:
			if !ok {
				t.Fatal("event stream ended")
			}
			if line != "" {
				return line
			}
		case <-deadline:
			t.Fatal("no event within the timeout")
		}
	}
}

// hello reads the opening event and returns its session token.
func (s *eventStream) hello(t *testing.T) string {
	t.Helper()
	if line := s.next(t, 5*time.Second); line != "event: hello" {
		t.Fatalf("first line = %q, want event: hello", line)
	}
	data := s.next(t, 5*time.Second)
	var hello struct {
		SessionToken string `json:"session_token"`
	}
	if !strings.HasPrefix(data, "data: ") || json.Unmarshal([]byte(strings.TrimPrefix(data, "data: ")), &hello) != nil {
		t.Fatalf("hello data = %q", data)
	}
	return hello.SessionToken
}

func (s *eventStream) close() {
	s.cancel()
	s.body.Close()
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
