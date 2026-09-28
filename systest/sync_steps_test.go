//go:build systest

package systest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

var demoUsers = []struct{ name, role string }{
	{"jvance", "admin"}, {"tmercer", "staff"}, {"lhayes", "nurse"}, {"mowens", "nurse"},
}

// Lists whose totals must match on both nodes after the first sync.
var parityLists = []string{
	"/api/patients", "/api/employees", "/api/products", "/api/allergies", "/api/suppliers",
	"/api/expenses", "/api/discounts", "/api/invoices", "/api/procedures",
}

func stepGateClosedBeforeConnect(t *testing.T, h *harness) {
	// Only the migration seed is on the cloud; its passwordless super-admin sets
	// a password at the first sign-in.
	h.cloudSuper = h.login(t, h.cloud, "super-admin", h.superPassword)
	r := h.cloudSuper.call(t, http.MethodPost, "/api/client-payments", map[string]any{})
	if r.status != http.StatusServiceUnavailable || r.env.Code != "sync_not_ready" {
		t.Fatalf("financial write before any clinic connected: %s, want 503 sync_not_ready", r)
	}
	if r := h.cloudSuper.call(t, http.MethodPost, "/api/allergies", map[string]any{}); r.status == http.StatusServiceUnavailable {
		t.Fatalf("a write outside the gate was refused by it: %s", r)
	}

	// The sync status answers the shared secret only.
	if r := h.machine(t, h.cloud, http.MethodGet, "/api/sync/status", map[string]string{"X-Sync-Secret": ""}, nil); r.status != http.StatusUnauthorized {
		t.Fatalf("sync status without the secret: %s, want 401", r)
	}
	r = h.machine(t, h.cloud, http.MethodGet, "/api/sync/status", nil, nil)
	var st struct {
		Self string `json:"self_id"`
	}
	if err := json.Unmarshal(r.body, &st); r.status != http.StatusOK || err != nil || st.Self != "cloud" {
		t.Fatalf("sync status: %s", r)
	}
	if n := len(h.proxy.since(0)); n != 0 {
		t.Fatalf("the proxy relayed %d requests while refusing connections", n)
	}
}

// stepDemoPush: the clinic's outbox keeps one entry per row, at the row's
// last write, so a 500-row push batch of the demo data carries product
// prices and invoice lines whose products and invoices were written later
// and sit in later batches. Each batch travels with those parents, so the
// demo data reaches an empty cloud on its own: no push is refused, both
// nodes hold the same data, the write gate opens and no failure notice
// appears.
func stepDemoPush(t *testing.T, h *harness) {
	h.clinicAdmin = h.login(t, h.clinic, "jvance", demoPassword)
	// The clinic's startup cycles failed while the proxy refused connections;
	// read that episode's notice so only new failures show up as unread.
	h.clinicAdmin.expect(t, http.StatusOK, http.MethodPut, "/api/notifications/read-all", nil)

	mark := h.proxy.mark()
	h.reconnect()
	eventually(t, 45*time.Second, "the demo data pushes to reach the cloud", func() (bool, string) {
		n := len(requests(h.proxy.since(mark), http.MethodPost, "/api/sync/push"))
		return n >= 2, fmt.Sprint(n)
	})
	eventually(t, 45*time.Second, "cloud write gate open after the demo pushes", func() (bool, string) {
		st, code := h.cloudSuper.gateStatus()
		return st == http.StatusBadRequest, fmt.Sprintf("%d %s", st, code)
	})

	if refused := logLines(h.cloud.currentLog(), "POST /api/sync/push -> 500"); len(refused) != 0 {
		t.Fatalf("refused pushes: %s", strings.Join(refused, "; "))
	}
	eventually(t, converge, "demo data parity between the nodes", func() (bool, string) {
		var diff []string
		for _, l := range parityLists {
			a, b := h.clinicAdmin.total(t, l, ""), h.cloudSuper.total(t, l, "")
			if a != b {
				diff = append(diff, fmt.Sprintf("%s: clinic %d, cloud %d", l, a, b))
			}
		}
		return len(diff) == 0, strings.Join(diff, "; ")
	})
	if n := len(unreadSyncFailures(h.clinicAdmin.notifications(t))); n != 0 {
		t.Fatalf("%d unread data sync failure notices on the clinic", n)
	}
	t.Logf("demo data pushed in %d push requests", len(requests(h.proxy.since(mark), http.MethodPost, "/api/sync/push")))
}

// stepInitialSync bootstraps the cloud the supported way, a cloud restore of
// the clinic's snapshot, then checks incremental sync both ways.
func stepInitialSync(t *testing.T, h *harness) {
	started := time.Now()
	r := h.clinicAdmin.call(t, http.MethodPost, "/api/cloud-restore", nil)
	if r.status != http.StatusOK || !r.env.Success {
		t.Fatalf("bootstrap cloud restore: %s", r)
	}
	var result struct {
		Tables int   `json:"tables"`
		Rows   int64 `json:"rows"`
	}
	r.into(t, &result)
	t.Logf("bootstrap restore: %d tables, %d rows in %s", result.Tables, result.Rows, time.Since(started).Round(100*time.Millisecond))

	// Every demo role signs in on both nodes; tokens are per node.
	for _, u := range demoUsers {
		for _, n := range []*node{h.clinic, h.cloud} {
			var s *session
			if u.name == "jvance" && n == h.clinic {
				s = h.clinicAdmin
			} else {
				s = h.login(t, n, u.name, demoPassword)
			}
			if s.role != u.role {
				t.Fatalf("%s on %s: role %q, want %q", u.name, n.name, s.role, u.role)
			}
			s.expect(t, http.StatusOK, http.MethodGet, "/api/auth/verify", nil)
			switch {
			case u.name == "jvance" && n == h.cloud:
				h.cloudAdmin = s
			case u.name == "lhayes" && n == h.cloud:
				h.cloudNurse = s
			}
		}
	}

	// The whole demo data set is on the cloud: list totals match.
	var parts []string
	for _, l := range parityLists {
		a, b := h.clinicAdmin.total(t, l, ""), h.cloudAdmin.total(t, l, "")
		if a != b {
			t.Fatalf("%s: clinic %d, cloud %d", l, a, b)
		}
		parts = append(parts, fmt.Sprintf("%s %d", strings.TrimPrefix(l, "/api/"), a))
	}
	t.Logf("demo data on the cloud: %s", strings.Join(parts, ", "))

	// With the outbox reset, the next cycle completes and opens the gate.
	h.waitGateOpen(t, 45*time.Second)

	// A cloud write reaches the clinic after a sync_pending event on the stream.
	mark := h.proxy.mark()
	name := "Systest cloud allergy"
	h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/allergies", map[string]any{"name": name})
	written := time.Now()
	eventually(t, converge, "cloud allergy on the clinic", func() (bool, string) {
		n, err := h.clinicAdmin.totalOrError("/api/allergies", name)
		return err == nil && n == 1, fmt.Sprint(n, err)
	})
	events := h.proxy.since(mark)
	pending := eventNames(events, "sync_pending")
	if len(pending) == 0 {
		t.Fatalf("no sync_pending event on the stream after the cloud write: %v", events)
	}
	if pulls := requests(after(events, pending[0].at), http.MethodGet, "/api/sync/pull"); len(pulls) == 0 {
		t.Fatalf("no pull after the sync_pending event: %v", events)
	}
	t.Logf("cloud write on the clinic after %s (event after %s)", time.Since(written).Round(10*time.Millisecond),
		pending[0].at.Sub(written).Round(10*time.Millisecond))

	// A clinic write reaches the cloud, users and password hashes included.
	p := h.createPatient(t, h.clinicAdmin, "Systest", "Initial")
	h.cloudAdmin.waitStatus(t, "/api/patients/"+p.ID, http.StatusOK)
	password := "St-" + randomText(12)
	h.clinicAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/users", map[string]any{
		"username": "systest-staff", "password": password, "displayName": "Systest Staff", "role": "staff",
	})
	eventually(t, converge, "new user on the cloud", func() (bool, string) {
		n, err := h.cloudAdmin.totalOrError("/api/users", "systest-staff")
		return err == nil && n == 1, fmt.Sprint(n, err)
	})
	if s := h.login(t, h.cloud, "systest-staff", password); s.role != "staff" {
		t.Fatalf("user made on the clinic signs in on the cloud as %q", s.role)
	}
}

func stepWriteGate(t *testing.T, h *harness) {
	// Open: a financial write passes; a nurse gets the scope error.
	p := h.createPatient(t, h.clinicAdmin, "Systest", "Gate")
	h.cloudAdmin.waitStatus(t, "/api/patients/"+p.ID, http.StatusOK)
	r := h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/client-payments",
		map[string]any{"patientId": p.ID, "amount": 25, "transactionMethod": "cash"})
	var pay struct {
		ID string `json:"id"`
	}
	r.into(t, &pay)
	if st, _ := h.cloudNurse.gateStatus(); st != http.StatusForbidden {
		t.Fatalf("nurse financial write with the gate open: %d, want 403", st)
	}
	h.waitPayment(t, h.clinicAdmin, p.ID, pay.ID)

	// The clinic stops: its event stream ends, the gate closes, and the 503
	// comes before the nurse's 403.
	h.clinic.stop(t)
	eventually(t, converge, "gate closed after the clinic stopped", func() (bool, string) {
		st, code := h.cloudAdmin.gateStatus()
		return st == http.StatusServiceUnavailable && code == "sync_not_ready", fmt.Sprint(st, code)
	})
	if st, code := h.cloudNurse.gateStatus(); st != http.StatusServiceUnavailable || code != "sync_not_ready" {
		t.Fatalf("nurse financial write with the gate closed: %d %s, want 503 sync_not_ready", st, code)
	}

	// The clinic comes back with a row in its outbox. The new session opens the
	// gate only after its first pull, push and ready, unless a poll cycle before
	// the session's events request already pushed the pending row. A narrower race
	// where a cycle starts before the events request and pushes after it
	// (events > push > ...) is not covered: accepting push before pull would loosen
	// the check against client ordering bugs.
	h.disconnect()
	h.clinic.start(t)
	pending := h.createPatient(t, h.clinicAdmin, "Systest", "Pending")
	if st, _ := h.cloudAdmin.gateStatus(); st != http.StatusServiceUnavailable {
		t.Fatalf("gate while the clinic can't connect: %d, want 503", st)
	}
	mark := h.proxy.mark()
	h.reconnect()
	h.waitGateOpen(t, converge)
	if err := checkHandshake(h.proxy.since(mark)); err != nil {
		t.Fatalf("session handshake: %s", err)
	}
	h.cloudAdmin.waitStatus(t, "/api/patients/"+pending.ID, http.StatusOK)
}

func (h *harness) waitPayment(t *testing.T, s *session, patientID, paymentID string) {
	t.Helper()
	eventually(t, converge, "payment on "+s.n.name, func() (bool, string) {
		ids, err := s.paymentIDs(patientID)
		if err != nil {
			return false, err.Error()
		}
		return contains(ids, paymentID), strings.Join(ids, ",")
	})
}

func (s *session) paymentIDs(patientID string) ([]string, error) {
	r, err := s.do(http.MethodGet, "/api/patients/"+patientID+"/payments?limit=100&filter=", nil)
	if err != nil {
		return nil, err
	}
	if r.status != http.StatusOK {
		return nil, fmt.Errorf("%s", r)
	}
	var d struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(r.env.Data, &d); err != nil {
		return nil, err
	}
	var ids []string
	for _, it := range d.Items {
		ids = append(ids, it.ID)
	}
	return ids, nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func after(events []proxyEvent, t time.Time) []proxyEvent {
	var out []proxyEvent
	for _, e := range events {
		if !e.at.Before(t) {
			out = append(out, e)
		}
	}
	return out
}

// checkHandshake judges the clinic's reconnect handshake in the proxy events
// since the reconnect. The new session starts at its GET /api/sync/events;
// requests before it belong to the old poll loop. From there up to the first
// POST /api/sync/ready the order must be events, pull, push, ready; requests
// after ready are not judged. The session may skip its push only when a POST
// /api/sync/push came before its events request: the old poll loop then
// already pushed the pending row.
func checkHandshake(events []proxyEvent) error {
	var before, session []proxyEvent
	for i, e := range events {
		if e.kind == "request" && e.method == http.MethodGet {
			path, _, _ := strings.Cut(e.path, "?")
			if path == "/api/sync/events" {
				before, session = events[:i], events[i:]
				break
			}
		}
	}
	if session == nil {
		return fmt.Errorf("no GET /api/sync/events request found")
	}

	var order []string
	for _, e := range session {
		if e.kind != "request" {
			continue
		}
		path, _, _ := strings.Cut(e.path, "?")
		step := e.method + " " + path
		if len(order) == 0 || order[len(order)-1] != step {
			order = append(order, step)
		}
		if path == "/api/sync/ready" {
			break
		}
	}

	want := "GET /api/sync/events > GET /api/sync/pull > POST /api/sync/push > POST /api/sync/ready"
	wantNoPush := "GET /api/sync/events > GET /api/sync/pull > POST /api/sync/ready"
	got := strings.Join(order, " > ")
	pushedBefore := len(requests(before, http.MethodPost, "/api/sync/push")) > 0

	if got == want || (pushedBefore && got == wantNoPush) {
		return nil
	}
	if pushedBefore {
		return fmt.Errorf("%s, want %s or %s", got, want, wantNoPush)
	}
	return fmt.Errorf("%s, want %s", got, want)
}

func TestCheckHandshake(t *testing.T) {
	req := func(method, path string) proxyEvent {
		return proxyEvent{kind: "request", method: method, path: path}
	}

	tests := []struct {
		name    string
		events  []proxyEvent
		wantErr bool
	}{
		{
			name: "the recorded race",
			events: []proxyEvent{
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/push"),
				req("GET", "/api/sync/events"),
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/ready"),
			},
			wantErr: false,
		},
		{
			name: "a clean handshake",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/push"),
				req("POST", "/api/sync/ready"),
			},
			wantErr: false,
		},
		{
			name: "a clean handshake followed by a later push",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/push"),
				req("POST", "/api/sync/ready"),
				req("POST", "/api/sync/push"),
			},
			wantErr: false,
		},
		{
			name: "push before pull",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("POST", "/api/sync/push"),
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/ready"),
			},
			wantErr: true,
		},
		{
			name: "ready before pull",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("POST", "/api/sync/ready"),
				req("GET", "/api/sync/pull"),
			},
			wantErr: true,
		},
		{
			name: "a missing ready",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/push"),
			},
			wantErr: true,
		},
		{
			name: "a second events after pull",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("GET", "/api/sync/pull"),
				req("GET", "/api/sync/events"),
				req("POST", "/api/sync/push"),
				req("POST", "/api/sync/ready"),
			},
			wantErr: true,
		},
		{
			name: "push after ready",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/ready"),
				req("POST", "/api/sync/push"),
			},
			wantErr: true,
		},
		{
			name: "no push without an earlier push",
			events: []proxyEvent{
				req("GET", "/api/sync/events"),
				req("GET", "/api/sync/pull"),
				req("POST", "/api/sync/ready"),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkHandshake(tc.events)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.name, err)
			}
		})
	}
}

func stepNewerWins(t *testing.T, h *harness) {
	p := h.createPatient(t, h.clinicAdmin, "Systest", "Newer")
	h.cloudAdmin.waitStatus(t, "/api/patients/"+p.ID, http.StatusOK)

	// The clinic's edit is newer: it wins on both nodes and the clinic logs local_wins.
	h.disconnect()
	c1 := h.cloudAdmin.editPatient(t, p.ID, "cloud edit 1")
	nextSecond(t, c1.UpdatedAt)
	l1 := h.clinicAdmin.editPatient(t, p.ID, "clinic edit 1")
	if l1.UpdatedAt <= c1.UpdatedAt {
		t.Fatalf("clinic edit %s is not newer than cloud edit %s", l1.UpdatedAt, c1.UpdatedAt)
	}
	h.reconnect()
	h.cloudAdmin.waitNotes(t, p.ID, "clinic edit 1")
	h.clinicAdmin.waitNotes(t, p.ID, "clinic edit 1")
	eventually(t, converge, "local_wins logged on the clinic", func() (bool, string) {
		return len(logLines(h.clinic.logText(), "local_wins", p.ID)) > 0, "no line"
	})

	// The cloud's edit is newer: it wins on both nodes.
	h.disconnect()
	l2 := h.clinicAdmin.editPatient(t, p.ID, "clinic edit 2")
	nextSecond(t, l2.UpdatedAt)
	h.cloudAdmin.editPatient(t, p.ID, "cloud edit 2")
	h.reconnect()
	h.clinicAdmin.waitNotes(t, p.ID, "cloud edit 2")
	h.cloudAdmin.waitNotes(t, p.ID, "cloud edit 2")

	// Both edits in the same second: the tie goes to the pulled (cloud) row.
	h.disconnect()
	want := ""
	for attempt := 1; want == ""; attempt++ {
		if attempt > 5 {
			t.Fatal("could not make two edits in the same second")
		}
		time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second + 20*time.Millisecond)))
		var wg sync.WaitGroup
		var lp, cp patient
		var lerr, cerr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			lp, lerr = h.clinicAdmin.editPatientErr(p.ID, fmt.Sprintf("clinic tie %d", attempt))
		}()
		go func() {
			defer wg.Done()
			cp, cerr = h.cloudAdmin.editPatientErr(p.ID, fmt.Sprintf("cloud tie %d", attempt))
		}()
		wg.Wait()
		if lerr != nil || cerr != nil {
			t.Fatalf("tie edits: %v, %v", lerr, cerr)
		}
		if lp.UpdatedAt == cp.UpdatedAt {
			want = cp.Notes
		}
	}
	h.reconnect()
	h.clinicAdmin.waitNotes(t, p.ID, want)
	h.cloudAdmin.waitNotes(t, p.ID, want)
}

func (s *session) editPatientErr(id, notes string) (patient, error) {
	r, err := s.do(http.MethodPut, "/api/patients/"+id, map[string]any{"notes": notes})
	if err != nil {
		return patient{}, err
	}
	if r.status != http.StatusOK {
		return patient{}, fmt.Errorf("%s", r)
	}
	var p patient
	err = json.Unmarshal(r.env.Data, &p)
	return p, err
}

func stepNoUpdatedAt(t *testing.T, h *harness) {
	r := h.clinicAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/products",
		map[string]any{"name": "Systest product", "unitPrice": 12, "quantity": 5})
	var prod struct {
		ID string `json:"id"`
	}
	r.into(t, &prod)
	h.cloudAdmin.waitStatus(t, "/api/products/"+prod.ID, http.StatusOK)

	// Products carry no updated_at: the pulled row wins even over a later clinic edit.
	h.disconnect()
	h.cloudAdmin.expect(t, http.StatusOK, http.MethodPut, "/api/products/"+prod.ID, map[string]any{"name": "Systest product cloud"})
	nextSecond(t, time.Now().UTC().Format(time.RFC3339))
	h.clinicAdmin.expect(t, http.StatusOK, http.MethodPut, "/api/products/"+prod.ID, map[string]any{"name": "Systest product clinic"})
	h.reconnect()
	for _, s := range []*session{h.clinicAdmin, h.cloudAdmin} {
		eventually(t, converge, "product name on "+s.n.name, func() (bool, string) {
			r, err := s.do(http.MethodGet, "/api/products/"+prod.ID, nil)
			if err != nil || r.status != http.StatusOK {
				return false, fmt.Sprint(r, err)
			}
			var d struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(r.env.Data, &d)
			return d.Name == "Systest product cloud", d.Name
		})
	}
}

type balance struct {
	ID       string  `json:"id"`
	Amount   float64 `json:"amount"`
	TotalIn  float64 `json:"totalIn"`
	TotalOut float64 `json:"totalOut"`
}

func (s *session) balance(kind, id string) (balance, error) {
	r, err := s.do(http.MethodGet, "/api/balances/"+kind+"/"+id, nil)
	if err != nil {
		return balance{}, err
	}
	if r.status != http.StatusOK {
		return balance{}, fmt.Errorf("%s", r)
	}
	var b balance
	err = json.Unmarshal(r.env.Data, &b)
	return b, err
}

func stepLedger(t *testing.T, h *harness) {
	p := h.createPatient(t, h.clinicAdmin, "Systest", "Ledger")
	r := h.clinicAdmin.expect(t, http.StatusOK, http.MethodGet, "/api/procedures/dropdown", nil)
	var procs []struct {
		ID string `json:"id"`
	}
	r.into(t, &procs)
	if len(procs) == 0 {
		t.Fatal("no procedure to invoice")
	}
	r = h.clinicAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/client-invoices", map[string]any{
		"patientId": p.ID,
		"items":     []map[string]any{{"itemType": "procedure", "itemId": procs[0].ID, "quantity": 1, "amount": 150}},
	})
	var inv struct {
		ID    string `json:"id"`
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	r.into(t, &inv)
	if len(inv.Items) != 1 {
		t.Fatalf("invoice items: %s", r)
	}
	r = h.clinicAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/client-payments",
		map[string]any{"patientId": p.ID, "amount": 100, "transactionMethod": "cash"})
	var pay struct {
		ID string `json:"id"`
	}
	r.into(t, &pay)
	h.cloudAdmin.waitStatus(t, "/api/invoices/"+inv.ID, http.StatusOK)
	h.waitPayment(t, h.cloudAdmin, p.ID, pay.ID)

	// With the gate open the cloud takes a payment of its own.
	h.waitGateOpen(t, converge)
	r = h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/client-payments",
		map[string]any{"patientId": p.ID, "amount": 50, "transactionMethod": "card"})
	var pay2 struct {
		ID string `json:"id"`
	}
	r.into(t, &pay2)
	h.waitPayment(t, h.clinicAdmin, p.ID, pay2.ID)

	// Balances recomputed from the applied transactions match on both nodes.
	for _, k := range []struct{ kind, id string }{{"patient", p.ID}, {"self", "self"}} {
		eventually(t, converge, k.kind+" balance equal on both nodes", func() (bool, string) {
			a, errA := h.clinicAdmin.balance(k.kind, k.id)
			b, errB := h.cloudAdmin.balance(k.kind, k.id)
			if errA != nil || errB != nil {
				return false, fmt.Sprint(errA, errB)
			}
			return a == b, fmt.Sprintf("clinic %+v cloud %+v", a, b)
		})
	}
	if b, err := h.cloudAdmin.balance("patient", p.ID); err != nil || b.Amount != 0 {
		t.Fatalf("patient balance after a 150 invoice and 100 + 50 paid: %+v %v, want 0", b, err)
	}
	a, _ := h.clinicAdmin.paymentIDs(p.ID)
	b, _ := h.cloudAdmin.paymentIDs(p.ID)
	if strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf("payment lists differ: clinic %v cloud %v", a, b)
	}
	h.ledger = ledgerRows{patient: p.ID, invoice: inv.ID, item: inv.Items[0].ID, payment: pay.ID}
}

func stepDeletes(t *testing.T, h *harness) {
	// A cloud delete wins on the clinic, even over a newer clinic edit.
	q := h.createPatient(t, h.clinicAdmin, "Systest", "Deleted")
	h.cloudAdmin.waitStatus(t, "/api/patients/"+q.ID, http.StatusOK)
	h.disconnect()
	h.cloudAdmin.expect(t, http.StatusOK, http.MethodDelete, "/api/patients/"+q.ID, nil)
	nextSecond(t, time.Now().UTC().Format(time.RFC3339))
	h.clinicAdmin.editPatient(t, q.ID, "edited after the cloud delete")
	h.reconnect()
	h.clinicAdmin.waitStatus(t, "/api/patients/"+q.ID, http.StatusNotFound)
	h.cloudAdmin.waitStatus(t, "/api/patients/"+q.ID, http.StatusNotFound)

	// Deletes of an expense (with its balance) and of a discount replicate.
	r := h.clinicAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/expenses", map[string]any{"name": "Systest expense"})
	var e struct {
		ID string `json:"id"`
	}
	r.into(t, &e)
	h.cloudAdmin.waitStatus(t, "/api/expenses/"+e.ID, http.StatusOK)
	h.cloudAdmin.waitStatus(t, "/api/balances/expense/"+e.ID, http.StatusOK)
	h.clinicAdmin.expect(t, http.StatusOK, http.MethodDelete, "/api/expenses/"+e.ID, nil)
	h.cloudAdmin.waitStatus(t, "/api/expenses/"+e.ID, http.StatusNotFound)
	h.cloudAdmin.waitStatus(t, "/api/balances/expense/"+e.ID, http.StatusNotFound)

	r = h.cloudAdmin.expect(t, http.StatusCreated, http.MethodPost, "/api/discounts", map[string]any{
		"name": "Systest offer", "discountType": "offer", "valueType": "percentage", "value": 5, "isActive": 1,
	})
	var d struct {
		ID string `json:"id"`
	}
	r.into(t, &d)
	h.clinicAdmin.waitStatus(t, "/api/discounts/"+d.ID, http.StatusOK)
	h.cloudAdmin.expect(t, http.StatusOK, http.MethodDelete, "/api/discounts/"+d.ID, nil)
	h.clinicAdmin.waitStatus(t, "/api/discounts/"+d.ID, http.StatusNotFound)

	// Deletes of invoices, invoice items, transactions and versions are refused
	// on both sides (no_delete). No API deletes them, so the rows go straight to
	// the sync protocol: pushed to the cloud, and pulled by the clinic from a fake peer.
	l := h.ledger
	now := time.Now().UTC().Format(time.RFC3339)
	rows := []syncRow{
		{Seq: 1, Table: "invoices", RowID: l.invoice, Op: "delete", CreatedAt: now},
		{Seq: 2, Table: "invoice_items", RowID: l.item, Op: "delete", CreatedAt: now},
		{Seq: 3, Table: "balance_transactions", RowID: l.payment, Op: "delete", CreatedAt: now},
		{Seq: 4, Table: "versions", RowID: newID(), Op: "delete", CreatedAt: now},
	}
	body, _ := json.Marshal(map[string]any{"rows": rows})
	r = h.machine(t, h.cloud, http.MethodPost, "/api/sync/push", nil, body)
	var res struct {
		AppliedSeq int64 `json:"applied_seq"`
		Conflicts  []struct {
			Table      string `json:"table"`
			RowID      string `json:"row_id"`
			Resolution string `json:"resolution"`
		} `json:"conflicts"`
	}
	if err := json.Unmarshal(r.body, &res); r.status != http.StatusOK || err != nil {
		t.Fatalf("push of refused deletes to the cloud: %s", r)
	}
	if len(res.Conflicts) != len(rows) {
		t.Fatalf("cloud conflicts: %s, want one no_delete per row", r)
	}
	// Deletes apply children first, so the conflicts come in that order.
	refused := map[string]string{}
	for _, c := range res.Conflicts {
		if c.Resolution == "no_delete" {
			refused[c.Table] = c.RowID
		}
	}
	for _, row := range rows {
		if refused[row.Table] != row.RowID {
			t.Fatalf("cloud conflicts %+v: no no_delete for %s %s", res.Conflicts, row.Table, row.RowID)
		}
	}
	h.checkLedgerRows(t, h.cloudAdmin)

	mark := h.proxy.mark()
	h.fake.serve(rows)
	h.proxy.setMode(modeFake)
	eventually(t, converge, "clinic pulled from the fake peer and finished the cycle", func() (bool, string) {
		served, ready, _ := h.fake.state()
		return served && ready > 0, fmt.Sprintf("served %v, ready calls %d", served, ready)
	})
	h.reconnect()
	_, _, pushed := h.fake.state()
	t.Logf("fake peer: %d requests, %d rows offered by the clinic's pushes", len(h.proxy.since(mark)), pushed)
	h.checkLedgerRows(t, h.clinicAdmin)
	log := h.clinic.logText()
	for _, row := range rows {
		if len(logLines(log, "no_delete", row.RowID)) == 0 {
			t.Fatalf("the clinic logged no no_delete conflict for %s %s", row.Table, row.RowID)
		}
	}
	h.waitGateOpen(t, 45*time.Second)
}

// checkLedgerRows asserts the ledger step's invoice, item and payment still exist.
func (h *harness) checkLedgerRows(t *testing.T, s *session) {
	t.Helper()
	l := h.ledger
	r := s.expect(t, http.StatusOK, http.MethodGet, "/api/invoices/"+l.invoice, nil)
	var inv struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	r.into(t, &inv)
	if len(inv.Items) != 1 || inv.Items[0].ID != l.item {
		t.Fatalf("invoice items on %s: %s", s.n.name, r)
	}
	ids, err := s.paymentIDs(l.patient)
	if err != nil || !contains(ids, l.payment) {
		t.Fatalf("payment %s on %s: %v %v", l.payment, s.n.name, ids, err)
	}
}
