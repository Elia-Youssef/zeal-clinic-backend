package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"clinic-api/client"
	"clinic-api/internal/database/store"
	"clinic-api/internal/monitor"

	"github.com/labstack/echo/v4"
)

// The contract suite sends every request through the full router and records
// it as a case in testdata/contract/<resource>.json: the request as sent and the
// normalized response. -update rewrites the files; otherwise any difference
// fails with a per-case diff.

const contractDir = "testdata/contract"

// unknownID matches no record: it is sent wherever a case needs an unknown id.
const unknownID = "00000000-0000-7000-8000-000000000000"

const (
	phaseScenario = "scenario"
	phaseCases    = "cases"
)

type contractActor struct {
	name  string
	token string
}

var anonymousActor = &contractActor{name: "anonymous"}

// contractCall is one request of the suite.
type contractCall struct {
	name        string                   // case name; "METHOD path" when empty
	file        string                   // golden file; the route's resource when empty
	method      string                   // defaults to GET
	path        string                   // path and query
	body        any                      // sent as JSON
	raw         *string                  // sent verbatim instead of body
	asCaller    *contractActor           // caller; the super-admin when nil
	header      map[string]string        // extra request headers
	capture     []string                 // response headers kept in the case
	orderedCase bool                     // keep the order of Data and Data.items
	keepOrder   []string                 // more arrays kept in the server's order, as ".Data.days"
	build       string                   // "clinic" or "cloud" for a case of one build only
	want        int                      // required status (scenario steps); 0 records any
	remote      string                   // client address; unique per request when empty
	stream      int                      // flushes to read from a streaming response
	utcDays     bool                     // response dates are UTC days (buckets of UTC timestamps)
	inspect     func(*contractReply) any // records this instead of the response body
}

type contractReply struct {
	status int
	header http.Header
	body   []byte
}

// data decodes the envelope's Data into out.
func (p *contractReply) data(t *testing.T, out any) {
	t.Helper()
	var env struct{ Data json.RawMessage }
	if err := json.Unmarshal(p.body, &env); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, p.body)
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		t.Fatalf("decode data: %v (%s)", err, env.Data)
	}
}

// id returns Data.id.
func (p *contractReply) id(t *testing.T) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	p.data(t, &v)
	if v.ID == "" {
		t.Fatalf("no id in %s", p.body)
	}
	return v.ID
}

// contractCase is one recorded request. A golden file lists the cases both
// builds share, then the clinic's own, then the cloud's own; each build
// rewrites its part and keeps the other's.
type contractCase struct {
	Name     string        `json:"name"`
	Build    string        `json:"build,omitempty"`
	Request  caseRequest   `json:"request"`
	Response *caseResponse `json:"response,omitempty"`
}

type caseRequest struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	As     string            `json:"as,omitempty"`
	Header map[string]string `json:"header,omitempty"`
	Body   any               `json:"body,omitempty"`
}

type caseResponse struct {
	Status int               `json:"status"`
	Header map[string]string `json:"header,omitempty"`
	Body   any               `json:"body,omitempty"`
	Text   string            `json:"text,omitempty"`
	Page   *pageCheck        `json:"page,omitempty"`
	File   *fileCheck        `json:"file,omitempty"`
}

// pageCheck describes an HTML response without storing it.
type pageCheck struct {
	ContentType string `json:"contentType"`
	IndexHTML   bool   `json:"indexHtml"`
}

// fileCheck describes a served file without storing its bytes.
type fileCheck struct {
	ContentType string `json:"contentType"`
	PDFHeader   bool   `json:"pdfHeader"`
	NonEmpty    bool   `json:"nonEmpty"`
}

type contractFile struct {
	name  string
	ids   *idNumbers
	cases []contractCase
	names map[string]int
}

type contractGoldenFile struct {
	Resource string         `json:"resource"`
	Cases    []contractCase `json:"cases"`
}

type routeCover struct {
	ok, clientErr int
}

type contractRun struct {
	t        *testing.T
	e        *echo.Echo
	cold     *echo.Echo
	super    contractActor
	norm     *contractNormalizer
	files    map[string]*contractFile
	order    []string
	resource map[string]string // "METHOD path" -> golden file
	build    map[string]string // "METHOD path" -> the only build with the route
	routes   []string          // registered "METHOD path" lines
	cover    map[string]*routeCover
	cache    *cacheCheck
	phase    string
	seq      int
}

// newContractRun opens a fresh database and the two servers under test and
// signs in the seeded super-admin (its first sign-in sets the password).
func newContractRun(t *testing.T) *contractRun {
	t.Helper()
	start := time.Now() // before the migrations, which stamp the seeded rows
	setupTestEnv(t)
	quietServerLogs(t)
	e := CreateServer()
	openCriticalSyncGate(t)
	r := &contractRun{
		t:        t,
		e:        e,
		cold:     CreateServer(),
		norm:     newContractNormalizer(t, start),
		files:    map[string]*contractFile{},
		resource: map[string]string{},
		build:    map[string]string{},
		cover:    map[string]*routeCover{},
		phase:    phaseScenario,
	}
	r.routes = routeInventory(e)
	for _, rule := range expectedRules(t) {
		r.resource[rule.key()] = goldenResourceOf(rule)
		r.build[rule.key()] = rule.Build
	}
	for _, line := range r.routes {
		if r.resource[line] == "" {
			t.Fatalf("route without a declared rule: %s", line)
		}
		r.cover[line] = &routeCover{}
	}
	r.cache = newCacheCheck(t)
	return r
}

// goldenResourceOf names the golden file of a route after the file that declares it.
func goldenResourceOf(rule routeRule) string {
	switch {
	case rule.Path == "/api/cloud-restore":
		return "cloud-restore"
	case strings.HasPrefix(rule.Origin, "routes/"):
		name := strings.TrimPrefix(rule.Origin, "routes/")
		name = name[:strings.Index(name, ".go")]
		return strings.ReplaceAll(name, "_", "-")
	case strings.HasPrefix(rule.Origin, "sync/"):
		return "sync"
	}
	return "server"
}

// route finds the registered route a request lands on: the one with the
// most literal segments among those that match.
func (r *contractRun) route(method, path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	segs := strings.Split(path, "/")
	best, bestScore := "", -1
	for _, line := range r.routes {
		m, pattern, _ := strings.Cut(line, " ")
		if m != method {
			continue
		}
		score, ok := matchRoutePattern(strings.Split(pattern, "/"), segs)
		if ok && score > bestScore {
			best, bestScore = line, score
		}
	}
	return best
}

func matchRoutePattern(pattern, segs []string) (int, bool) {
	score := 0
	for i, p := range pattern {
		if p == "*" {
			return score, true
		}
		if i >= len(segs) {
			return 0, false
		}
		switch {
		case strings.HasPrefix(p, ":"):
			if segs[i] == "" {
				return 0, false
			}
		case p == segs[i]:
			score++
		default:
			return 0, false
		}
	}
	return score, len(pattern) == len(segs)
}

func (r *contractRun) file(name string) *contractFile {
	f := r.files[name]
	if f == nil {
		f = &contractFile{name: name, ids: newIDNumbers(), names: map[string]int{}}
		r.files[name] = f
		r.order = append(r.order, name)
	}
	return f
}

// run sends one request, records it and, for a successful write of the
// scenario, checks the response cache. A case of the other build is skipped
// and returns nil.
func (r *contractRun) run(c contractCall) *contractReply {
	r.t.Helper()
	if c.method == "" {
		c.method = http.MethodGet
	}
	if c.build != "" && c.build != buildName() {
		return nil
	}
	key := r.route(c.method, c.path)
	fileName := c.file
	if fileName == "" {
		fileName = r.resource[key]
	}
	if fileName == "" {
		fileName = "server"
	}
	f := r.file(fileName)
	name := c.name
	if name == "" {
		name = c.method + " " + c.path
	}
	rep := r.send(c)
	if c.want != 0 && rep.status != c.want {
		r.t.Fatalf("%s %s: status %d, want %d: %s", c.method, c.path, rep.status, c.want, rep.body)
	}
	cs := contractCase{Name: r.caseName(f, name), Build: c.build}
	cs.Request = r.caseRequest(f, c)
	cs.Response = r.caseResponse(f, c, rep)
	f.add(cs)
	if key != "" {
		cv := r.cover[key]
		switch {
		case rep.status < 300:
			cv.ok++
		case rep.status < 500:
			cv.clientErr++
		}
	}
	if r.phase == phaseScenario && c.method != http.MethodGet && rep.status < 300 && !strings.HasPrefix(c.path, "/api/auth/") {
		r.cache.check(r, fileName+": "+cs.Name)
	}
	return rep
}

// caseName makes names unique within a file.
func (r *contractRun) caseName(f *contractFile, name string) string {
	name = r.norm.text(f.ids, name)
	f.names[name]++
	if n := f.names[name]; n > 1 {
		return fmt.Sprintf("%s #%d", name, n)
	}
	return name
}

func (f *contractFile) add(c contractCase) { f.cases = append(f.cases, c) }

// send serves the request on the primary server.
func (r *contractRun) send(c contractCall) *contractReply {
	r.t.Helper()
	return r.serve(r.e, c)
}

func (r *contractRun) serve(e *echo.Echo, c contractCall) *contractReply {
	r.t.Helper()
	var body io.Reader
	switch {
	case c.raw != nil:
		body = strings.NewReader(*c.raw)
	case c.body != nil:
		b, err := json.Marshal(c.body)
		if err != nil {
			r.t.Fatalf("marshal body: %v", err)
		}
		body = bytes.NewReader(b)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, c.method, c.path, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	who := r.super
	if c.asCaller != nil {
		who = *c.asCaller
	}
	if who.token != "" {
		req.Header.Set("Authorization", "Bearer "+who.token)
	}
	for k, v := range c.header {
		req.Header.Set(k, v)
	}
	r.seq++
	req.RemoteAddr = c.remote
	if req.RemoteAddr == "" {
		req.RemoteAddr = fmt.Sprintf("10.%d.%d.%d:40000", r.seq>>16&0xff, r.seq>>8&0xff, r.seq&0xff)
	}
	rec := &streamRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, flushes: c.stream}
	e.ServeHTTP(rec, req)
	// Low-stock checks run after the response; wait so their notifications
	// are in place before the next request.
	monitor.WaitAsync()
	return &contractReply{status: rec.Code, header: rec.Header(), body: rec.Body.Bytes()}
}

// streamRecorder ends a streaming response after a number of flushes.
type streamRecorder struct {
	*httptest.ResponseRecorder
	cancel  context.CancelFunc
	flushes int
	seen    int
}

func (s *streamRecorder) Flush() {
	s.ResponseRecorder.Flush()
	s.seen++
	if s.flushes > 0 && s.seen >= s.flushes {
		s.cancel()
	}
}

func (r *contractRun) caseRequest(f *contractFile, c contractCall) caseRequest {
	cr := caseRequest{Method: c.method, Path: r.norm.text(f.ids, c.path)}
	if c.asCaller != nil {
		cr.As = c.asCaller.name
	}
	if len(c.header) > 0 {
		cr.Header = map[string]string{}
		for k, v := range c.header {
			switch {
			case strings.EqualFold(k, "X-Sync-Secret") || strings.EqualFold(k, "X-Publish-Secret"):
				v = secretLabel(v)
			case strings.EqualFold(k, "X-Sync-Version") && v == r.norm.version:
				v = "<version>"
			}
			cr.Header[k] = r.norm.text(f.ids, v)
		}
	}
	switch {
	case c.raw != nil:
		cr.Body = *c.raw
	case c.body != nil:
		b, _ := json.Marshal(c.body)
		cr.Body = r.norm.asSent(f.ids, decodeContractJSON(r.t, b))
	}
	return cr
}

// secretLabel names a machine secret instead of recording it.
func secretLabel(v string) string {
	switch v {
	case testSyncSecret:
		return "<sync secret>"
	case testPublishSecret:
		return "<publish secret>"
	case "":
		return ""
	}
	return "<wrong secret>"
}

func (r *contractRun) caseResponse(f *contractFile, c contractCall, rep *contractReply) *caseResponse {
	cr := &caseResponse{Status: rep.status}
	for _, h := range c.capture {
		if v := rep.header.Get(h); v != "" {
			if cr.Header == nil {
				cr.Header = map[string]string{}
			}
			cr.Header[h] = r.norm.text(f.ids, v)
		}
	}
	ct := rep.header.Get(echo.HeaderContentType)
	norm := r.norm
	if c.utcDays {
		norm = r.norm.utc()
	}
	switch {
	case c.inspect != nil:
		cr.Body = r.norm.normalize(f.ids, c.inspect(rep), nil)
	case len(rep.body) == 0:
	case strings.HasPrefix(ct, "application/json"):
		keepOrder := c.keepOrder
		if c.orderedCase {
			keepOrder = append([]string{".Data", ".Data.items"}, keepOrder...)
		}
		cr.Body = norm.normalize(f.ids, decodeContractJSON(r.t, rep.body), keepOrder)
	case strings.HasPrefix(ct, "text/html"):
		cr.Page = &pageCheck{ContentType: ct, IndexHTML: bytes.Equal(rep.body, spaIndexPage(r.t))}
	case strings.HasPrefix(ct, "application/pdf"), strings.HasPrefix(ct, "application/octet-stream"):
		cr.File = &fileCheck{ContentType: ct, PDFHeader: bytes.HasPrefix(rep.body, []byte("%PDF-")), NonEmpty: len(rep.body) > 0}
	default:
		cr.Text = r.norm.text(f.ids, string(rep.body))
	}
	return cr
}

// spaIndexPage is the page the server answers for client-side routes.
func spaIndexPage(t *testing.T) []byte {
	t.Helper()
	b, err := fs.ReadFile(client.DistFS(), "index.html")
	if err != nil {
		return []byte(`<!doctype html><meta charset="utf-8"><title>Zeal Clinic</title><body>Frontend not built.</body>`)
	}
	return b
}

func decodeContractJSON(t *testing.T, b []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode JSON: %v (%s)", err, b)
	}
	return v
}

// rawJSONKeys lists the keys of a JSON object in the order they were written.
func rawJSONKeys(t *testing.T, b []byte) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		t.Fatalf("not a JSON object: %s", b)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// checkCoverage requires a case for every registered route: a successful one
// for each GET, and both a successful and a client-error one for each write,
// except where the reason below says the outcome can't be produced here.
func (r *contractRun) checkCoverage(noSuccess, noClientError map[string]string) {
	r.t.Helper()
	var missing []string
	for _, line := range r.routes {
		cv := r.cover[line]
		if cv.ok == 0 && cv.clientErr == 0 {
			missing = append(missing, line+": no case")
			continue
		}
		if cv.ok == 0 && noSuccess[line] == "" {
			missing = append(missing, line+": no successful case")
		}
		if !strings.HasPrefix(line, "GET ") && cv.clientErr == 0 && noClientError[line] == "" {
			missing = append(missing, line+": no client-error case")
		}
	}
	for _, m := range missing {
		r.t.Error("coverage: " + m)
	}
}

// finish compares every file with its golden file, or rewrites them with -update.
func (r *contractRun) finish() {
	r.t.Helper()
	for _, name := range r.order {
		f := r.files[name]
		r.t.Run(name, func(t *testing.T) { r.checkFile(t, f) })
	}
	r.checkUnproduced()
}

// casesOfBuild returns the cases of one build ("" for the shared ones) in order.
func casesOfBuild(cases []contractCase, build string) []contractCase {
	var out []contractCase
	for _, c := range cases {
		if c.Build == build {
			out = append(out, c)
		}
	}
	return out
}

func (r *contractRun) checkFile(t *testing.T, f *contractFile) {
	path := filepath.Join(packageDir, filepath.FromSlash(contractDir), f.name+".json")
	old, readErr := readContractGolden(path)
	this := buildName()
	got := append(casesOfBuild(f.cases, ""), casesOfBuild(f.cases, this)...)
	if *updateGoldens {
		out := contractGoldenFile{Resource: f.name}
		for _, b := range []string{"", "clinic", "cloud"} {
			switch {
			case b == "" || b == this:
				out.Cases = append(out.Cases, casesOfBuild(f.cases, b)...)
			case old != nil:
				out.Cases = append(out.Cases, casesOfBuild(old.Cases, b)...)
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encodeContractGolden(t, out), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s/%s.json", contractDir, f.name)
		return
	}
	if readErr != nil {
		t.Fatalf("read golden %s/%s.json: %v (create it with -update)", contractDir, f.name, readErr)
	}
	want := append(casesOfBuild(old.Cases, ""), casesOfBuild(old.Cases, this)...)
	if msg := diffCases(t, want, got); msg != "" {
		t.Errorf("%s/%s.json does not match (update it with -update only for a deliberate change):\n%s", contractDir, f.name, msg)
	}
}

// checkUnproduced fails for golden files that hold cases of this build that
// the run no longer produces.
func (r *contractRun) checkUnproduced() {
	r.t.Helper()
	dir := filepath.Join(packageDir, filepath.FromSlash(contractDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, ent := range entries {
		name, ok := strings.CutSuffix(ent.Name(), ".json")
		if !ok || r.files[name] != nil || !isResourceGolden(name) {
			continue
		}
		g, err := readContractGolden(filepath.Join(dir, ent.Name()))
		if err != nil {
			r.t.Errorf("%s/%s: %v", contractDir, ent.Name(), err)
			continue
		}
		for _, c := range g.Cases {
			if c.Build == "" || c.Build == buildName() {
				r.t.Errorf("%s/%s holds cases this run no longer produces; remove the file deliberately", contractDir, ent.Name())
				break
			}
		}
	}
}

// isResourceGolden tells the per-resource files apart from the other
// contract files in the folder.
func isResourceGolden(name string) bool {
	return name != cacheGoldenName
}

func readContractGolden(path string) (*contractGoldenFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g contractGoldenFile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&g); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &g, nil
}

func encodeContractGolden(t *testing.T, v any) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// diffCases lists the cases that differ, with the lines around the first
// difference of each.
func diffCases(t *testing.T, want, got []contractCase) string {
	t.Helper()
	var b strings.Builder
	shown := 0
	for i := 0; i < max(len(want), len(got)); i++ {
		var w, g string
		label := ""
		if i < len(want) {
			w = string(encodeContractGolden(t, want[i]))
			label = want[i].Name
		}
		if i < len(got) {
			g = string(encodeContractGolden(t, got[i]))
			label = got[i].Name
		}
		if w == g {
			continue
		}
		if shown == 8 {
			b.WriteString("... more cases differ\n")
			break
		}
		shown++
		switch {
		case w == "":
			fmt.Fprintf(&b, "case %d %q: not in the golden file\n", i+1, label)
		case g == "":
			fmt.Fprintf(&b, "case %d %q: no longer produced\n", i+1, label)
		default:
			fmt.Fprintf(&b, "case %d %q:\n%s", i+1, label, windowDiff(w, g, 12))
		}
	}
	if len(want) != len(got) {
		fmt.Fprintf(&b, "(%d cases in the golden file, %d produced)\n", len(want), len(got))
	}
	return b.String()
}

// windowDiff shows the differing middle part of two texts, after their
// common leading and trailing lines.
func windowDiff(want, got string, limit int) string {
	wl := strings.Split(strings.TrimSuffix(want, "\n"), "\n")
	gl := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	pre := 0
	for pre < len(wl) && pre < len(gl) && wl[pre] == gl[pre] {
		pre++
	}
	suf := 0
	for suf < len(wl)-pre && suf < len(gl)-pre && wl[len(wl)-1-suf] == gl[len(gl)-1-suf] {
		suf++
	}
	var b strings.Builder
	write := func(sign string, lines []string) {
		for i, l := range lines {
			if i == limit {
				fmt.Fprintf(&b, "  %s ... (%d more lines)\n", sign, len(lines)-limit)
				return
			}
			fmt.Fprintf(&b, "  %s%s\n", sign, l)
		}
	}
	if pre > 0 {
		fmt.Fprintf(&b, "    %s\n", strings.TrimSpace(wl[pre-1]))
	}
	write("-", wl[pre:len(wl)-suf])
	write("+", gl[pre:len(gl)-suf])
	return b.String()
}

// login signs in and returns the caller.
func (r *contractRun) login(name, username, password string) *contractActor {
	r.t.Helper()
	rep := r.run(contractCall{name: "sign in as " + name, method: http.MethodPost, path: "/api/auth/login",
		body: map[string]string{"username": username, "password": password}, asCaller: anonymousActor, want: http.StatusOK})
	var v struct {
		Token string `json:"token"`
	}
	rep.data(r.t, &v)
	return &contractActor{name: name, token: v.Token}
}

// seededIDs lists the seeded ids a fresh database always holds; responses keep
// them as they are.
func seededIDs(t *testing.T) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for _, table := range []string{"procedure_types", "procedure_categories", "procedures", "procedure_prices",
		"rooms", "balances", "countries", "lebanon_cities", "users"} {
		rows, err := store.RDB.Query("SELECT id FROM " + table)
		if err != nil {
			t.Fatalf("read seeded ids of %s: %v", table, err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			ids[id] = true
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []string{"USD", "LBP"} {
		ids[store.DeterministicID("balances", "self", "self", c)] = true
	}
	ids[unknownID] = true
	return ids
}

func sortedMapKeys[M ~map[string]V, V any](m M) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
