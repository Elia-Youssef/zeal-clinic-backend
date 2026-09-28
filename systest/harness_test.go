//go:build systest

package systest

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	converge = 30 * time.Second
	// streamReconnect bounds a wait on the clinic's event stream: it reconnects
	// after at most 30 s of backoff, and the cycle it starts has to finish too.
	streamReconnect = 45 * time.Second
	demoPassword    = "demo123"
)

type harness struct {
	cfg    config
	clinic *node
	cloud  *node
	alt    *node
	proxy  *faultProxy
	fake   *fakePeer
	fakeHS *http.Server
	client *http.Client

	superPassword string

	// Sessions, signed in once per node (sign-in is rate limited per address).
	cloudSuper  *session
	clinicAdmin *session
	cloudAdmin  *session
	cloudNurse  *session

	// Rows made by one step and used by a later one.
	ledger ledgerRows
	phone  int
}

type ledgerRows struct {
	patient string
	invoice string
	item    string
	payment string
}

func newHarness(t *testing.T, cfg config) *harness {
	t.Helper()
	h := &harness{
		cfg:           cfg,
		client:        &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 8}},
		superPassword: "St-" + randomText(12),
		phone:         70100000,
	}
	t.Cleanup(h.close)
	var err error
	if h.cloud, err = newNode("cloud", cfg.cloudBin, cfg.cloudPort, filepath.Join(cfg.workDir, "cloud"), "data"); err != nil {
		t.Fatal(err)
	}
	if h.clinic, err = newNode("clinic", cfg.clinicBin, cfg.clinicPort, filepath.Join(cfg.workDir, "clinic"), "tmp"); err != nil {
		t.Fatal(err)
	}
	if cfg.altCloudBin != "" {
		if h.alt, err = newNode("cloud-alt", cfg.altCloudBin, cfg.altCloudPort, filepath.Join(cfg.workDir, "cloud-alt"), "data"); err != nil {
			t.Fatal(err)
		}
	}
	if h.proxy, err = startProxy(cfg.proxyPort, h.cloud.addr()); err != nil {
		t.Fatal(err)
	}
	h.fake = &fakePeer{h: h}
	h.fakeHS = &http.Server{Handler: h.fake, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = h.fakeHS.Serve(h.proxy.fake) }()

	// The clinic starts unable to reach its peer, so the first step sees the
	// cloud before any clinic has connected.
	h.proxy.setMode(modeRefuse)
	h.cloud.start(t)
	h.clinic.runOnce(t, "seed.log", "--dev", "--seed-only", "--demo")
	h.clinic.start(t)
	return h
}

// close ends whatever still runs after a failure; stepShutdown checks the
// graceful stop.
func (h *harness) close() {
	for _, n := range []*node{h.clinic, h.cloud, h.alt} {
		if n != nil {
			n.abort()
		}
	}
	if h.fakeHS != nil {
		_ = h.fakeHS.Close()
	}
	if h.proxy != nil {
		h.proxy.close()
	}
}

func (h *harness) disconnect() { h.proxy.setMode(modeRefuse) }

func (h *harness) reconnect() { h.proxy.setMode(modeForward) }

func randomText(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, n)
	for i := range b {
		v, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[v.Int64()]
	}
	return string(b)
}

// newID returns a random UUID-shaped id for rows the test invents.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// eventually polls check until it reports true or the timeout passes.
func eventually(t *testing.T, timeout time.Duration, what string, check func() (bool, string)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ok, detail := check()
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: not reached within %s (last: %s)", what, timeout, detail)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// nextSecond waits until the wall clock is in a later second than stamp, so the
// next write gets a strictly newer updated_at (the column has 1 s resolution).
func nextSecond(t *testing.T, stamp string) {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		t.Fatalf("parse updatedAt %q: %v", stamp, err)
	}
	for !time.Now().UTC().Truncate(time.Second).After(ts) {
		time.Sleep(20 * time.Millisecond)
	}
}

type reply struct {
	status int
	body   []byte
	env    envelope
}

// envelope covers both response shapes: {Success, Error, Data} from the API and
// {success, error, data} or {error, code} from the machine endpoints.
type envelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data"`
}

func (r reply) String() string {
	s := strings.TrimSpace(string(r.body))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return fmt.Sprintf("%d %s", r.status, s)
}

func (r reply) into(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.env.Data, v); err != nil {
		t.Fatalf("decode %s: %v", r, err)
	}
}

func (h *harness) send(method, url string, header map[string]string, body []byte) (reply, error) {
	return h.sendWith(h.client, method, url, header, body)
}

// sendWith is send through another client, for a call that outlasts the
// shared client's timeout.
func (h *harness) sendWith(client *http.Client, method, url string, header map[string]string, body []byte) (reply, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return reply{}, err
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return reply{}, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	r := reply{status: resp.StatusCode, body: b}
	_ = json.Unmarshal(b, &r.env)
	return r, err
}

// machine calls a node's shared-secret endpoints the way the peer does.
func (h *harness) machine(t *testing.T, n *node, method, path string, header map[string]string, body []byte) reply {
	t.Helper()
	hd := map[string]string{"X-Sync-Secret": h.cfg.syncSecret, "X-Sync-Version": "dev"}
	for k, v := range header {
		hd[k] = v
	}
	for k, v := range hd {
		if v == "" {
			delete(hd, k)
		}
	}
	r, err := h.send(method, n.url(path), hd, body)
	if err != nil {
		t.Fatalf("%s %s on %s: %v", method, path, n.name, err)
	}
	return r
}

type session struct {
	h     *harness
	n     *node
	user  string
	role  string
	token string
}

func (h *harness) login(t *testing.T, n *node, user, password string) *session {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"username": user, "password": password})
	r, err := h.send(http.MethodPost, n.url("/api/auth/login"), nil, b)
	if err != nil {
		t.Fatalf("sign in %s on %s: %v", user, n.name, err)
	}
	if r.status != http.StatusOK {
		t.Fatalf("sign in %s on %s: %s", user, n.name, r)
	}
	var d struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	r.into(t, &d)
	if d.Token == "" {
		t.Fatalf("sign in %s on %s: no token", user, n.name)
	}
	return &session{h: h, n: n, user: user, role: d.Role, token: d.Token}
}

func (s *session) do(method, path string, body any) (reply, error) {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	return s.h.send(method, s.n.url(path), map[string]string{"Authorization": "Bearer " + s.token}, b)
}

// doWithin is do with its own deadline instead of the shared client's.
func (s *session) doWithin(timeout time.Duration, method, path string, body any) (reply, error) {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	client := &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	return s.h.sendWith(client, method, s.n.url(path), map[string]string{"Authorization": "Bearer " + s.token}, b)
}

func (s *session) call(t *testing.T, method, path string, body any) reply {
	t.Helper()
	r, err := s.do(method, path, body)
	if err != nil {
		t.Fatalf("%s %s on %s as %s: %v", method, path, s.n.name, s.user, err)
	}
	return r
}

func (s *session) expect(t *testing.T, want int, method, path string, body any) reply {
	t.Helper()
	r := s.call(t, method, path, body)
	if r.status != want {
		t.Fatalf("%s %s on %s as %s: got %s, want %d", method, path, s.n.name, s.user, r, want)
	}
	return r
}

// status is the answer's status code, or 0 when the node didn't answer.
func (s *session) status(method, path string, body any) int {
	r, err := s.do(method, path, body)
	if err != nil {
		return 0
	}
	return r.status
}

// total reads {items, total} from a list endpoint; the filter parameter keeps
// the answer out of the response cache.
func (s *session) total(t *testing.T, path, filter string) int {
	t.Helper()
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	r := s.expect(t, http.StatusOK, http.MethodGet, path+sep+"limit=1&filter="+urlEscape(filter), nil)
	var d struct {
		Total int `json:"total"`
	}
	r.into(t, &d)
	return d.Total
}

func (s *session) totalOrError(path, filter string) (int, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	r, err := s.do(http.MethodGet, path+sep+"limit=1&filter="+urlEscape(filter), nil)
	if err != nil {
		return 0, err
	}
	if r.status != http.StatusOK {
		return 0, fmt.Errorf("%s", r)
	}
	var d struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(r.env.Data, &d); err != nil {
		return 0, err
	}
	return d.Total, nil
}

func urlEscape(s string) string {
	return strings.NewReplacer("%", "%25", " ", "%20", "&", "%26", "+", "%2B", "#", "%23", "?", "%3F").Replace(s)
}

type patient struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Notes     string `json:"notes"`
	UpdatedAt string `json:"updatedAt"`
}

func (h *harness) createPatient(t *testing.T, s *session, first, last string) patient {
	t.Helper()
	h.phone++
	r := s.expect(t, http.StatusCreated, http.MethodPost, "/api/patients", map[string]any{
		"firstName": first, "lastName": last, "gender": "Female", "contact": strconv.Itoa(h.phone),
	})
	var p patient
	r.into(t, &p)
	return p
}

func (s *session) patient(id string) (patient, int) {
	r, err := s.do(http.MethodGet, "/api/patients/"+id, nil)
	if err != nil {
		return patient{}, 0
	}
	var p patient
	if r.status == http.StatusOK {
		_ = json.Unmarshal(r.env.Data, &p)
	}
	return p, r.status
}

func (s *session) editPatient(t *testing.T, id, notes string) patient {
	t.Helper()
	r := s.expect(t, http.StatusOK, http.MethodPut, "/api/patients/"+id, map[string]any{"notes": notes})
	var p patient
	r.into(t, &p)
	return p
}

// waitStatus polls GET path on the session's node until it answers want.
func (s *session) waitStatus(t *testing.T, path string, want int) {
	t.Helper()
	eventually(t, converge, fmt.Sprintf("GET %s on %s answers %d", path, s.n.name, want), func() (bool, string) {
		st := s.status(http.MethodGet, path, nil)
		return st == want, strconv.Itoa(st)
	})
}

// waitNotes polls a patient on the session's node until its notes match.
func (s *session) waitNotes(t *testing.T, id, want string) {
	t.Helper()
	eventually(t, converge, fmt.Sprintf("patient notes %q on %s", want, s.n.name), func() (bool, string) {
		p, st := s.patient(id)
		return st == http.StatusOK && p.Notes == want, fmt.Sprintf("%d %q", st, p.Notes)
	})
}

// gateStatus probes the cloud write gate without writing: an empty payment is
// refused by the gate (503) or, once the gate is open, by validation (400).
func (s *session) gateStatus() (int, string) {
	r, err := s.do(http.MethodPost, "/api/client-payments", map[string]any{})
	if err != nil {
		return 0, err.Error()
	}
	return r.status, r.env.Code
}

func (h *harness) waitGateOpen(t *testing.T, timeout time.Duration) {
	t.Helper()
	eventually(t, timeout, "cloud write gate open", func() (bool, string) {
		st, code := h.cloudAdmin.gateStatus()
		return st == http.StatusBadRequest, fmt.Sprintf("%d %s", st, code)
	})
}

type notification struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Action string `json:"action"`
	IsRead bool   `json:"isRead"`
}

func (s *session) notifications(t *testing.T) []notification {
	t.Helper()
	r := s.expect(t, http.StatusOK, http.MethodGet, "/api/notifications?limit=100&filter=", nil)
	var d struct {
		Items []notification `json:"items"`
	}
	r.into(t, &d)
	return d.Items
}

func unreadSyncFailures(list []notification) []notification {
	var out []notification
	for _, n := range list {
		if n.Action == "sync-failure" && !n.IsRead {
			out = append(out, n)
		}
	}
	return out
}

// syncRow is one sync_log entry on the wire.
type syncRow struct {
	Seq       int64           `json:"seq"`
	Table     string          `json:"table"`
	RowID     string          `json:"row_id"`
	Op        string          `json:"op"`
	RowJSON   json.RawMessage `json:"row_json,omitempty"`
	UpdatedAt string          `json:"updated_at,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// fakePeer answers the clinic's sync calls when the proxy is in fake mode: one
// pull hands out the rows set by serve, pushes are refused so the clinic keeps
// its outbox, and ready calls are counted.
type fakePeer struct {
	h          *harness
	mu         sync.Mutex
	rows       []syncRow
	served     bool
	readyAfter int
	pushed     int
}

func (f *fakePeer) serve(rows []syncRow) {
	f.mu.Lock()
	f.rows, f.served, f.readyAfter, f.pushed = rows, false, 0, 0
	f.mu.Unlock()
}

func (f *fakePeer) state() (served bool, readyAfter, pushed int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.served, f.readyAfter, f.pushed
}

func (f *fakePeer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.h.proxy.record(proxyEvent{kind: "request", method: r.Method, path: "fake " + r.URL.RequestURI()})
	if r.Header.Get("X-Sync-Secret") != f.h.cfg.syncSecret {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/api/sync/events":
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "event: hello\ndata: {\"session_token\":\"fake-session\"}\n\n")
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-r.Context().Done()
	case "/api/sync/pull":
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		rows := []syncRow{}
		f.mu.Lock()
		if !f.served && len(f.rows) > 0 {
			for _, row := range f.rows {
				row.Seq = since
				rows = append(rows, row)
			}
			f.served = true
		}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows, "max_seq": since})
	case "/api/sync/push":
		var req struct {
			Rows []json.RawMessage `json:"rows"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.Rows) > 0 {
			f.mu.Lock()
			f.pushed += len(req.Rows)
			f.mu.Unlock()
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"the fake peer keeps no rows"}`)
			return
		}
		fmt.Fprint(w, `{"applied_seq":0}`)
	case "/api/sync/ready":
		f.mu.Lock()
		if f.served {
			f.readyAfter++
		}
		f.mu.Unlock()
		fmt.Fprint(w, `{"ready":true}`)
	case "/api/sync/failed":
		fmt.Fprint(w, `{"ready":false}`)
	default:
		http.NotFound(w, r)
	}
}

// requests lists the request lines of events, optionally only one method and
// path prefix.
func requests(events []proxyEvent, method, prefix string) []proxyEvent {
	var out []proxyEvent
	for _, e := range events {
		if e.kind == "request" && (method == "" || e.method == method) && strings.HasPrefix(e.path, prefix) {
			out = append(out, e)
		}
	}
	return out
}

func eventNames(events []proxyEvent, name string) []proxyEvent {
	var out []proxyEvent
	for _, e := range events {
		if e.kind == "event" && e.path == name {
			out = append(out, e)
		}
	}
	return out
}
