package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The parity tests hold the dashboard to the backend: its scope list, the
// endpoints it calls and the envelope keys it reads. They read the dashboard
// source from DASHBOARD_DIR and skip when it is not set.

func dashboardDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("DASHBOARD_DIR")
	if dir == "" {
		t.Skip("DASHBOARD_DIR is not set: the parity tests need a dashboard checkout")
	}
	if _, err := os.Stat(filepath.Join(dir, "src", "lib", "api.ts")); err != nil {
		t.Fatalf("DASHBOARD_DIR=%s is not a dashboard checkout: %v", dir, err)
	}
	return dir
}

func readDashboardFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

var scopeLiteralRe = regexp.MustCompile(`"([a-z-]+:[a-z]+)"`)

// The super-admin holds every scope, and the dashboard's ALL_SCOPES must name
// exactly those.
func TestParityScopes(t *testing.T) {
	dir := dashboardDir(t)
	src := readDashboardFile(t, dir, "src/lib/scopes.ts")
	start := strings.Index(src, "export const ALL_SCOPES = [")
	end := strings.Index(src[max(start, 0):], "] as const")
	if start < 0 || end < 0 {
		t.Fatal("ALL_SCOPES not found in src/lib/scopes.ts")
	}
	var want []string
	for _, m := range scopeLiteralRe.FindAllStringSubmatch(src[start:start+end], -1) {
		want = append(want, m[1])
	}

	setupTestEnv(t)
	quietServerLogs(t)
	e := newTestServer(t)
	rec := doRequest(t, e, http.MethodGet, "/api/auth/me", nil, adminToken(t, e))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/auth/me: %d %s", rec.Code, rec.Body.String())
	}
	var me struct {
		Scopes []string `json:"scopes"`
	}
	decodeEnvelope(t, rec.Body, &me)

	missing, extra := stringSetDiff(me.Scopes, want)
	for _, s := range missing {
		t.Errorf("the super-admin holds %q, which ALL_SCOPES lacks", s)
	}
	for _, s := range extra {
		t.Errorf("ALL_SCOPES lists %q, which the backend does not grant", s)
	}
	if dup := duplicateStrings(want); len(dup) > 0 {
		t.Errorf("ALL_SCOPES lists these twice: %v", dup)
	}
	t.Logf("%d scopes on both sides", len(want))
}

// stringSetDiff returns the items only in a and only in b.
func stringSetDiff(a, b []string) (onlyA, onlyB []string) {
	for _, x := range a {
		if !slices.Contains(b, x) {
			onlyA = append(onlyA, x)
		}
	}
	for _, x := range b {
		if !slices.Contains(a, x) {
			onlyB = append(onlyB, x)
		}
	}
	return onlyA, onlyB
}

func duplicateStrings(items []string) []string {
	seen := map[string]int{}
	var out []string
	for _, x := range items {
		seen[x]++
		if seen[x] == 2 {
			out = append(out, x)
		}
	}
	return out
}

// apiCall is one call site of the dashboard's api client.
type apiCall struct {
	file   string
	line   int
	fn     string // get, post, put, patch, del or openPdf
	method string
	path   string // "/api/..." with :param segments, empty when dynamic
	expr   string // the first argument as written
}

func (c apiCall) key() string { return c.fn + " " + c.file + ": " + c.expr }

// dynamicCall lists the endpoints a call site builds at run time: fixed ones,
// and those a pattern finds in the dashboard source.
type dynamicCall struct {
	endpoints []string
	source    *regexp.Regexp
}

// dynamicCalls are the call sites whose endpoint is built at run time, keyed
// by function, file and first argument.
var dynamicCalls = map[string]dynamicCall{
	// List pages pass their endpoint to the list component.
	"get src/components/data/data-list.tsx: `${endpoint}${sep}${params}`": {source: regexp.MustCompile(`\bendpoint="(/[^"?]*)`)},
	// The analytics panels and cards name the endpoint the analytics hook
	// loads: an /analytics/ path literal, bare, in withRange or as a prop.
	"get src/components/analytics/use-analytics.ts: endpoint": {source: regexp.MustCompile(`"(/analytics/[^"?]*)`)},
	// A line's price comes from its product or procedure.
	"get src/components/forms/client-invoice-form.tsx: endpoint": {endpoints: []string{"/products/:param", "/procedures/:param"}},
	// The adjustment and write-off routes of each balance type.
	"post src/components/forms/balance-adjustment-form.tsx: endpoint": {source: regexp.MustCompile(`\b(?:adjustment|writeOff): "(/[^"]+)"`)},
	// An invoice is a client or a supplier invoice.
	"put src/pages/financials/invoice-detail.tsx: `/${prefix}/${id}`": {endpoints: []string{"/client-invoices/:param", "/supplier-invoices/:param"}},
	"del src/pages/financials/invoice-detail.tsx: `/${prefix}/${id}`": {endpoints: []string{"/client-invoices/:param", "/supplier-invoices/:param"}},
}

// Every endpoint the dashboard calls exists in the backend's route list.
func TestParityEndpoints(t *testing.T) {
	dir := dashboardDir(t)
	raw, err := os.ReadFile(filepath.Join(packageDir, "testdata", "routes", buildName()+".txt"))
	if err != nil {
		t.Fatalf("read the route list: %v", err)
	}
	var routes []string
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if line != "" {
			routes = append(routes, line)
		}
	}

	calls := scanDashboardCalls(t, dir)
	if len(calls) < 100 {
		t.Fatalf("found only %d api calls in the dashboard; the scanner no longer understands the source", len(calls))
	}
	sources := dashboardSources(t, dir)
	used := map[string]bool{}
	checked := 0
	check := func(c apiCall, path string) {
		checked++
		if !slices.ContainsFunc(routes, func(r string) bool { return dashboardRouteMatches(r, c.method, path) }) {
			t.Errorf("%s:%d: api.%s calls %s %s, which is not a backend route", c.file, c.line, c.fn, c.method, path)
		}
	}
	for _, c := range calls {
		if c.path != "" {
			check(c, c.path)
			continue
		}
		d, ok := dynamicCalls[c.key()]
		if !ok {
			t.Errorf("%s:%d: api.%s with an endpoint built at run time (%s); list it in dynamicCalls", c.file, c.line, c.fn, c.expr)
			continue
		}
		used[c.key()] = true
		targets := slices.Clone(d.endpoints)
		if d.source != nil {
			for _, src := range sources {
				for _, m := range d.source.FindAllStringSubmatch(src, -1) {
					if p := cleanEndpoint(m[1]); p != "" && !slices.Contains(targets, p) {
						targets = append(targets, p)
					}
				}
			}
		}
		if len(targets) == 0 {
			t.Errorf("%s:%d: dynamicCalls names no endpoint for %q", c.file, c.line, c.key())
		}
		for _, p := range targets {
			check(c, "/api"+p)
		}
	}
	for k := range dynamicCalls {
		if !used[k] {
			t.Errorf("dynamicCalls lists %q, which is no longer in the dashboard", k)
		}
	}
	t.Logf("%d call sites, %d endpoints checked against %d routes", len(calls), checked, len(routes))
}

// dashboardSources reads every .ts and .tsx file of the dashboard's src folder.
func dashboardSources(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(filepath.Join(dir, "src"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !(strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, strings.ReplaceAll(string(b), "\r\n", "\n"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// dashboardRouteMatches compares a route line with a call, a :param on either side
// matching any one segment.
func dashboardRouteMatches(route, method, path string) bool {
	m, pattern, _ := strings.Cut(route, " ")
	if m != method {
		return false
	}
	ps, cs := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(ps) != len(cs) {
		return false
	}
	for i := range ps {
		if ps[i] != cs[i] && !strings.HasPrefix(ps[i], ":") && cs[i] != ":param" {
			return false
		}
	}
	return true
}

var apiCallRe = regexp.MustCompile(`\bapi\.(get|post|put|patch|del|openPdf)\b`)

var callMethods = map[string]string{
	"get": http.MethodGet, "post": http.MethodPost, "put": http.MethodPut,
	"patch": http.MethodPatch, "del": http.MethodDelete, "openPdf": http.MethodGet,
}

// scanDashboardCalls finds every api.get/post/put/patch/del/openPdf call in
// the dashboard's src folder and reads its endpoint argument.
func scanDashboardCalls(t *testing.T, dir string) []apiCall {
	t.Helper()
	root := filepath.Join(dir, "src")
	var out []apiCall
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !(strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := strings.ReplaceAll(string(b), "\r\n", "\n")
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		for _, loc := range apiCallRe.FindAllStringSubmatchIndex(src, -1) {
			method := src[loc[2]:loc[3]]
			i := skipGeneric(src, loc[1])
			i = skipScriptSpace(src, i)
			if i >= len(src) || src[i] != '(' {
				continue // a reference, not a call
			}
			expr, endpoint := firstArgument(src, skipScriptSpace(src, i+1))
			c := apiCall{file: rel, line: strings.Count(src[:loc[0]], "\n") + 1, fn: method, method: callMethods[method], expr: expr}
			// A path whose first segment is built at run time names no route.
			if endpoint != "" && !strings.HasPrefix(endpoint, "/:param") {
				c.path = "/api" + endpoint
			}
			out = append(out, c)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].file != out[b].file {
			return out[a].file < out[b].file
		}
		return out[a].line < out[b].line
	})
	return out
}

func skipScriptSpace(src string, i int) int {
	for i < len(src) && strings.ContainsRune(" \t\n", rune(src[i])) {
		i++
	}
	return i
}

// skipGeneric steps over a type argument list such as <Paginated<T> | T[]>.
func skipGeneric(src string, i int) int {
	j := skipScriptSpace(src, i)
	if j >= len(src) || src[j] != '<' {
		return i
	}
	depth := 0
	for ; j < len(src); j++ {
		switch src[j] {
		case '<':
			depth++
		case '>':
			if j > 0 && src[j-1] == '=' {
				continue // an arrow inside a function type
			}
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return i
}

// firstArgument reads a call's first argument and, when it is a string or
// template literal (possibly wrapped in a helper call), the endpoint it
// names: template parts that fill a whole segment become :param, and a query
// string or a suffix built at run time is dropped.
func firstArgument(src string, i int) (expr, endpoint string) {
	end := argumentEnd(src, i)
	expr = strings.Join(strings.Fields(src[i:end]), " ")
	if i >= len(src) {
		return expr, ""
	}
	switch src[i] {
	case '"', '\'':
		j := strings.IndexByte(src[i+1:], src[i])
		if j < 0 {
			return expr, ""
		}
		if next := skipScriptSpace(src, i+2+j); next < len(src) && src[next] == '+' {
			return expr, ""
		}
		return expr, cleanEndpoint(src[i+1 : i+1+j])
	case '`':
		return expr, templateEndpoint(src, i+1)
	}
	// A helper such as withRange("/path", ...): read its first argument.
	j := i
	for j < len(src) && (src[j] == '_' || src[j] == '.' || src[j] >= 'a' && src[j] <= 'z' || src[j] >= 'A' && src[j] <= 'Z' || src[j] >= '0' && src[j] <= '9') {
		j++
	}
	if j > i && j < len(src) && src[j] == '(' {
		if _, inner := firstArgument(src, skipScriptSpace(src, j+1)); inner != "" {
			return expr, inner
		}
	}
	return expr, ""
}

// argumentEnd finds the comma or parenthesis that ends the argument at i.
func argumentEnd(src string, i int) int {
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth == 0 {
				return j
			}
			depth--
		case ',':
			if depth == 0 {
				return j
			}
		case '"', '\'', '`':
			if k := strings.IndexByte(src[j+1:], src[j]); k >= 0 {
				j += k + 1
			}
		}
	}
	return len(src)
}

// templateEndpoint converts the template literal starting at i.
func templateEndpoint(src string, i int) string {
	var b strings.Builder
	for i < len(src) {
		switch {
		case src[i] == '`':
			return cleanEndpoint(b.String())
		case strings.HasPrefix(src[i:], "${"):
			depth, j := 0, i+1
			for ; j < len(src); j++ {
				if src[j] == '{' {
					depth++
				} else if src[j] == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			cur := b.String()
			next := byte('`')
			if j+1 < len(src) {
				next = src[j+1]
			}
			if (cur == "" || strings.HasSuffix(cur, "/")) && (next == '/' || next == '?' || next == '`') {
				if cur == "" {
					return "" // the whole path is built at run time
				}
				b.WriteString(":param")
			} else {
				return cleanEndpoint(cur) // a suffix built at run time
			}
			i = j + 1
		default:
			b.WriteByte(src[i])
			i++
		}
	}
	return ""
}

func cleanEndpoint(p string) string {
	if k := strings.IndexByte(p, '?'); k >= 0 {
		p = p[:k]
	}
	if !strings.HasPrefix(p, "/") {
		return ""
	}
	return strings.TrimSuffix(p, "/")
}

var (
	envelopeReadRe = regexp.MustCompile(`\bjson\.([A-Z][A-Za-z]*)\b`)
	paginatedRe    = regexp.MustCompile(`export type Paginated<T> = \{([^}]*)\}`)
	fieldNameRe    = regexp.MustCompile(`(\w+)\??:`)
)

// The keys the dashboard client reads from a response are the keys of the
// recorded envelope.
func TestParityEnvelope(t *testing.T) {
	dir := dashboardDir(t)
	src := readDashboardFile(t, dir, "src/lib/api.ts")
	var reads []string
	for _, m := range envelopeReadRe.FindAllStringSubmatch(src, -1) {
		if !slices.Contains(reads, m[1]) {
			reads = append(reads, m[1])
		}
	}
	m := paginatedRe.FindStringSubmatch(src)
	if m == nil || len(reads) == 0 {
		t.Fatal("the envelope handling in src/lib/api.ts changed shape; update this test")
	}
	var listKeys []string
	for _, f := range fieldNameRe.FindAllStringSubmatch(m[1], -1) {
		listKeys = append(listKeys, f[1])
	}

	g, err := readContractGolden(filepath.Join(packageDir, filepath.FromSlash(contractDir), "envelope.json"))
	if err != nil {
		t.Fatalf("read the envelope golden: %v", err)
	}
	keysOf := func(name, field string) []string {
		for _, c := range g.Cases {
			if c.Name != name || c.Response == nil {
				continue
			}
			var v map[string][]string
			b, _ := json.Marshal(c.Response.Body)
			if err := json.Unmarshal(b, &v); err != nil {
				t.Fatalf("envelope case %q: %v", name, err)
			}
			return v[field]
		}
		t.Fatalf("envelope case %q not found", name)
		return nil
	}

	for _, name := range []string{"a record", "a list", "a not-found error", "a validation error", "a sign-in error"} {
		keys := keysOf(name, "keys")
		if onlyClient, onlyServer := stringSetDiff(reads, keys); len(onlyClient) > 0 || len(onlyServer) > 0 {
			t.Errorf("%s: the client reads %v, the envelope has %v", name, reads, keys)
		}
	}
	if got := keysOf("a list", "dataKeys"); !slices.Equal(sortedStrings(got), sortedStrings(listKeys)) {
		t.Errorf("a list: Paginated<T> has %v, the list data has %v", listKeys, got)
	}
	t.Logf("envelope keys %v, list keys %v", reads, listKeys)
}

func sortedStrings(s []string) []string {
	c := slices.Clone(s)
	slices.Sort(c)
	return c
}
