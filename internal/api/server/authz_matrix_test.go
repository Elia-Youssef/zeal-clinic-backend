package server

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"clinic-api/internal/buildmode"

	"github.com/labstack/echo/v4"
)

// Outcome of one probe: the guard that stopped the request, or pass when no
// guard did, whatever the handler then answered.
const (
	outcomeNoAuth  = "401"
	outcomeNoScope = "403-scope"
	outcomeGate    = "503-gate"
	outcomeVersion = "409-version"
	outcomePass    = "pass"
)

const (
	probeMissingID = "00000000-0000-7000-8000-000000000000"
	probeVersion   = "0.0.0-probe"
	gateOpenSuffix = " (gate open)"
)

// principal is one kind of caller.
type principal struct {
	Name     string
	Token    string
	Scopes   []string          // the role's scopes, for signed-in callers
	Employee string            // own employee id, sent on scope-or-self routes
	User     string            // own user id, sent on scope-or-self routes
	Headers  map[string]string // machine headers
	Query    string            // sync secret sent as the sync_secret query parameter
}

func (p principal) selfID(kind string) string {
	switch kind {
	case "employee":
		return p.Employee
	case "user":
		return p.User
	}
	return ""
}

// expectOutcome evaluates a route's declared guards, in order, for a caller.
func expectOutcome(r routeRule, p principal, gateOpen bool) string {
	for _, g := range r.Guards {
		switch g.Kind {
		case guardJWT:
			if p.Token == "" {
				return outcomeNoAuth
			}
		case guardScope:
			if !slices.Contains(p.Scopes, g.Scopes[0]) {
				return outcomeNoScope
			}
		case guardAnyScope:
			if !slices.ContainsFunc(g.Scopes, func(s string) bool { return slices.Contains(p.Scopes, s) }) {
				return outcomeNoScope
			}
		case guardAllScopes:
			for _, s := range g.Scopes {
				if !slices.Contains(p.Scopes, s) {
					return outcomeNoScope
				}
			}
		case guardScopeOrSelf:
			if !slices.Contains(p.Scopes, g.Scopes[0]) && p.selfID(g.Self) == "" {
				return outcomeNoScope
			}
		case guardCritical:
			if buildmode.Cloud && !gateOpen {
				return outcomeGate
			}
		case guardSyncSecret:
			secret := p.Headers["X-Sync-Secret"]
			if secret == "" {
				secret = p.Query
			}
			if secret != testSyncSecret {
				return outcomeNoAuth
			}
		case guardSyncHeader:
			if p.Headers["X-Sync-Secret"] != testSyncSecret {
				return outcomeNoAuth
			}
		case guardPublish:
			if p.Headers["X-Publish-Secret"] != testPublishSecret {
				return outcomeNoAuth
			}
		case guardVersion:
			if p.Headers["X-Sync-Version"] != buildmode.Version {
				return outcomeVersion
			}
		}
	}
	return outcomePass
}

// classify names the guard that produced a response, by status and body.
func classify(code int, body string) string {
	switch code {
	case http.StatusUnauthorized:
		for _, signature := range []string{
			`"Error":"Please sign in again"`, // bearer token
			`"error":"unauthorized"`,         // sync secret
			`"Error":"Not authorized"`,       // publish and peer-update secrets
			`"error":"Not authorized"`,       // cloud-restore secret
		} {
			if strings.Contains(body, signature) {
				return outcomeNoAuth
			}
		}
	case http.StatusForbidden:
		if strings.Contains(body, `"Error":"You don't have permission"`) {
			return outcomeNoScope
		}
	case http.StatusServiceUnavailable:
		if strings.Contains(body, `"code":"sync_not_ready"`) {
			return outcomeGate
		}
	case http.StatusConflict:
		if strings.Contains(strings.ToLower(body), "version mismatch") {
			return outcomeVersion
		}
	}
	return outcomePass
}

// probeRecorder cancels the request once the handler flushes, so the
// streaming endpoints return after their first event.
type probeRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r *probeRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.cancel()
}

// probePath fills in the path parameters: the caller's own id on
// scope-or-self routes, ids that match no record everywhere else.
func probePath(r routeRule, p principal) string {
	self := ""
	for _, g := range r.Guards {
		if g.Kind == guardScopeOrSelf {
			self = p.selfID(g.Self)
		}
	}
	segs := strings.Split(r.Path, "/")
	for i, s := range segs {
		switch {
		case s == ":id" && self != "":
			segs[i] = self
		case s == ":type":
			segs[i] = "employee"
		case s == ":name":
			segs[i] = "missing-role"
		case strings.HasPrefix(s, ":"):
			segs[i] = probeMissingID
		case s == "*" && strings.HasPrefix(r.Path, "/files/"):
			segs[i] = "missing.pdf"
		case s == "*":
			segs[i] = "some/deep-link"
		}
	}
	path := strings.Join(segs, "/")
	if p.Query != "" {
		path += "?sync_secret=" + url.QueryEscape(p.Query)
	}
	return path
}

// probe sends one request as p and classifies the answer. Writes carry an
// empty JSON object. Each probe comes from its own address, so the login
// rate limit never applies.
func probe(e *echo.Echo, r routeRule, p principal, seq int) string {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var body io.Reader
	if r.Method != http.MethodGet {
		body = strings.NewReader("{}")
	}
	req := httptest.NewRequestWithContext(ctx, r.Method, probePath(r, p), body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	req.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:40000", seq>>16&0xff, seq>>8&0xff, seq&0xff)
	rec := &probeRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
	e.ServeHTTP(rec, req)
	return classify(rec.Code, rec.Body.String())
}

// newAuthzPrincipals signs in one user per role (the staff and nurse users
// are linked to employees) and returns every kind of caller of this build.
func newAuthzPrincipals(t *testing.T, e *echo.Echo) []principal {
	t.Helper()
	superTok := loginAdmin(t, e, "probe-super-pw")
	createUserAccount(t, e, superTok, "probe-admin", "admin", "probe-admin-pw")
	staffEmp, staffUser := createLinkedEmployee(t, e, superTok, "Probe", "Staff", "probe-staff", "staff", "probe-staff-pw")
	nurseEmp, nurseUser := createLinkedEmployee(t, e, superTok, "Probe", "Nurse", "probe-nurse", "nurse", "probe-nurse-pw")
	adminTok := loginAs(t, e, "probe-admin", "probe-admin-pw")
	staffTok := loginAs(t, e, "probe-staff", "probe-staff-pw")
	nurseTok := loginAs(t, e, "probe-nurse", "probe-nurse-pw")
	scopes := roleScopes(t)

	ps := []principal{
		{Name: "anonymous"},
		{Name: "super-admin", Token: superTok, Scopes: scopes["super-admin"]},
		{Name: "admin", Token: adminTok, Scopes: scopes["admin"]},
		{Name: "staff", Token: staffTok, Scopes: scopes["staff"]},
		{Name: "nurse", Token: nurseTok, Scopes: scopes["nurse"]},
		{Name: "staff-self", Token: staffTok, Scopes: scopes["staff"], Employee: staffEmp, User: staffUser},
		{Name: "nurse-self", Token: nurseTok, Scopes: scopes["nurse"], Employee: nurseEmp, User: nurseUser},
	}
	if !buildmode.Cloud {
		return ps
	}
	v := buildmode.Version
	return append(ps,
		principal{Name: "no-secret", Headers: map[string]string{"X-Sync-Version": v}},
		principal{Name: "wrong-secret", Headers: map[string]string{
			"X-Sync-Secret": "wrong-secret", "X-Publish-Secret": "wrong-secret", "X-Sync-Version": v}},
		principal{Name: "wrong-version", Headers: map[string]string{"X-Sync-Secret": testSyncSecret, "X-Sync-Version": probeVersion}},
		principal{Name: "sync-secret", Headers: map[string]string{"X-Sync-Secret": testSyncSecret, "X-Sync-Version": v}},
		principal{Name: "query-secret", Headers: map[string]string{"X-Sync-Version": v}, Query: testSyncSecret},
		principal{Name: "publish-secret", Headers: map[string]string{"X-Publish-Secret": testPublishSecret}},
	)
}

// checkRulesCoverRoutes: every registered route has exactly one declared rule
// on this build, and every declared route is registered.
func checkRulesCoverRoutes(t *testing.T, rules []routeRule, inventory []string) {
	t.Helper()
	declared := map[string]int{}
	for _, r := range rules {
		declared[r.key()]++
	}
	registered := map[string]bool{}
	for _, line := range inventory {
		registered[line] = true
		if declared[line] == 0 {
			t.Errorf("registered route without a declared rule: %s", line)
		}
	}
	for _, r := range rules {
		if declared[r.key()] > 1 {
			t.Errorf("route declared more than once: %s (%s)", r.key(), r.Origin)
		}
		if !registered[r.key()] {
			t.Errorf("declared route is not registered: %s (%s)", r.key(), r.Origin)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
}

type matrixRow struct {
	key      string // "METHOD path", marked when probed with the gate open
	rule     routeRule
	outcomes []string
}

// checkOrderings asserts the check order whatever the declarations say:
// sign-in comes before scopes and the sync gate, on the cloud the sync gate
// comes before scopes, and a bearer token never opens a machine route.
func checkOrderings(t *testing.T, principals []principal, rows []matrixRow) {
	t.Helper()
	for _, row := range rows {
		machine := row.rule.has(guardSyncSecret) || row.rule.has(guardSyncHeader) || row.rule.has(guardPublish)
		gateClosed := buildmode.Cloud && row.rule.has(guardCritical) && !strings.HasSuffix(row.key, gateOpenSuffix)
		for i, p := range principals {
			got := row.outcomes[i]
			switch {
			case row.rule.has(guardJWT) && p.Token == "" && got != outcomeNoAuth:
				t.Errorf("%s as %s: %s, want 401 before any other check", row.key, p.Name, got)
			case gateClosed && p.Token != "" && got != outcomeGate:
				t.Errorf("%s as %s: %s, want 503-gate before the scope check", row.key, p.Name, got)
			case machine && p.Token != "" && got != outcomeNoAuth:
				t.Errorf("%s as %s: %s, want 401: machine routes ignore bearer tokens", row.key, p.Name, got)
			}
		}
	}
}

// TestAuthzMatrix sends one request per route and caller and checks the
// outcome twice: against the declared guards (a wiring check) and against the
// golden table (a declaration check). On the cloud the critical-sync gate is
// closed, as on a cloud no clinic has synced with; the gated routes are
// probed again once it is open.
func TestAuthzMatrix(t *testing.T) {
	setupTestEnv(t)
	quietServerLogs(t)
	e := CreateServer()
	principals := newAuthzPrincipals(t, e)
	rules := expectedRules(t)
	checkRulesCoverRoutes(t, rules, routeInventory(e))

	var rows []matrixRow
	var mismatches []string
	seq := 0
	start := time.Now()
	run := func(r routeRule, gateOpen bool) {
		row := matrixRow{key: r.key(), rule: r, outcomes: make([]string, len(principals))}
		if gateOpen {
			row.key += gateOpenSuffix
		}
		for i, p := range principals {
			seq++
			got := probe(e, r, p, seq)
			if want := expectOutcome(r, p, gateOpen); got != want {
				mismatches = append(mismatches, fmt.Sprintf("%s as %s: got %s, the declared guards %q give %s (%s)",
					row.key, p.Name, got, r.describe(), want, r.Origin))
			}
			row.outcomes[i] = got
		}
		rows = append(rows, row)
	}
	// Signing out revokes the caller's token, so that route goes last.
	var last []routeRule
	for _, r := range rules {
		if r.key() == "POST /api/auth/logout" {
			last = append(last, r)
			continue
		}
		run(r, false)
	}
	if buildmode.Cloud {
		closeGate := openCriticalSyncGate(t)
		for _, r := range rules {
			if r.has(guardCritical) {
				run(r, true)
			}
		}
		closeGate()
	}
	for _, r := range last {
		run(r, false)
	}
	t.Logf("%s build: %d rows x %d callers = %d probes in %s",
		buildName(), len(rows), len(principals), seq, time.Since(start).Round(time.Millisecond))

	for i, m := range mismatches {
		if i == 60 {
			t.Errorf("... and %d more", len(mismatches)-i)
			break
		}
		t.Error(m)
	}
	checkOrderings(t, principals, rows)
	if t.Failed() && *updateGoldens {
		t.Fatal("golden not rewritten: the outcomes break the declared guards or the check order")
	}

	// Rows by path, then method, the gate-open row after the gate-closed one.
	slices.SortFunc(rows, func(a, b matrixRow) int {
		return cmp.Or(cmp.Compare(a.rule.Path, b.rule.Path), cmp.Compare(a.rule.Method, b.rule.Method), cmp.Compare(a.key, b.key))
	})
	var b strings.Builder
	b.WriteString("route\tguard")
	for _, p := range principals {
		b.WriteString("\t" + p.Name)
	}
	b.WriteString("\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", row.key, row.rule.describe(), strings.Join(row.outcomes, "\t"))
	}
	checkGolden(t, "testdata/authz/"+buildName()+".tsv", b.String())
}
