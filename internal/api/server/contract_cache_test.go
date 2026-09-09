package server

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	mw "clinic-api/internal/api/middleware"
	"clinic-api/internal/database/store"
)

// cacheCheck looks for stale responses: after every scenario write it reads
// each cacheable GET once through the primary server, whose cache the write
// should have invalidated, and once through a second server instance with the
// cache emptied. The response cache is process-wide, so emptying it is what
// makes the second instance start cold. Reads that differ are recorded in
// testdata/contract/cache.json.

const cacheGoldenName = "cache"

type staleWrite struct {
	Write string   `json:"write"`
	Stale []string `json:"stale"`
}

type cacheGolden struct {
	Probes []string     `json:"probes"`
	Stale  []staleWrite `json:"stale"`
}

type cacheCheck struct {
	routes map[string]bool // cached "GET path" -> whether it caches every URL
	stale  []staleWrite
	probes func() []string
	writes int
	reads  int
}

func newCacheCheck(t *testing.T) *cacheCheck {
	t.Helper()
	return &cacheCheck{routes: cachedRoutes(t)}
}

// cacheable reports whether the response cache can serve url: routes with
// the cache middleware, unless the path has parameters or the query holds
// "filter", which the non-forcing variant skips.
func (cc *cacheCheck) cacheable(r *contractRun, url string) bool {
	key := r.route(http.MethodGet, url)
	force, ok := cc.routes[key]
	if !ok {
		return false
	}
	return force || (!strings.Contains(key, "/:") && !strings.Contains(url, "filter"))
}

func (cc *cacheCheck) check(r *contractRun, write string) {
	r.t.Helper()
	if cc.probes == nil {
		return
	}
	urls := cc.probes()
	cc.writes++
	warm := make([][]byte, len(urls))
	for i, u := range urls {
		warm[i] = r.serve(r.e, contractCall{method: http.MethodGet, path: u}).body
	}
	mw.InvalidateCacheAll()
	var stale []string
	for i, u := range urls {
		if cold := r.serve(r.cold, contractCall{method: http.MethodGet, path: u}).body; !bytes.Equal(warm[i], cold) {
			stale = append(stale, u)
		}
	}
	cc.reads += 2 * len(urls)
	if len(stale) > 0 {
		cc.stale = append(cc.stale, staleWrite{Write: write, Stale: stale})
	}
}

// selfTest proves the check notices a stale read: a change written to the
// database behind the server's back must show up as one. The change is
// undone afterwards.
func (cc *cacheCheck) selfTest(r *contractRun) {
	r.t.Helper()
	const url = "/api/rooms?sort=name&order=asc"
	rename := func(name string) {
		if _, err := store.DB.Exec(`UPDATE rooms SET name = ? WHERE id = ?`, name, seedRoom4); err != nil {
			r.t.Fatal(err)
		}
	}
	probes, stale, writes, reads := cc.probes, len(cc.stale), cc.writes, cc.reads
	cc.probes = func() []string { return []string{url} }
	r.serve(r.e, contractCall{method: http.MethodGet, path: url})
	rename("Room 4 renamed behind the cache")
	cc.check(r, "self-test")
	found := len(cc.stale) == stale+1
	rename("Room 4")
	mw.InvalidateCacheAll()
	cc.probes, cc.stale, cc.writes, cc.reads = probes, cc.stale[:stale], writes, reads
	if !found {
		r.t.Fatal("the cache check missed a stale read; it can't be trusted")
	}
}

// finish compares the stale reads with testdata/contract/cache.json.
func (cc *cacheCheck) finish(r *contractRun) {
	r.t.Helper()
	r.t.Run(cacheGoldenName, func(t *testing.T) {
		t.Logf("cache: %d writes checked, %d reads", cc.writes, cc.reads)
		ids := newIDNumbers()
		g := cacheGolden{Probes: []string{}, Stale: []staleWrite{}}
		for _, u := range cc.probes() {
			g.Probes = append(g.Probes, r.norm.text(ids, u))
		}
		for _, s := range cc.stale {
			w := staleWrite{Write: s.Write}
			for _, u := range s.Stale {
				w.Stale = append(w.Stale, r.norm.text(ids, u))
			}
			g.Stale = append(g.Stale, w)
		}
		got := string(encodeContractGolden(t, g))
		checkGolden(t, contractDir+"/"+cacheGoldenName+".json", got)
	})
}

// cachedRoutes finds the GET routes declared in internal/api/routes with the
// cache middleware, and whether it is the variant that caches every URL.
func cachedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join(packageDir, "..", "routes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read route declarations: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, ent := range entries {
		name := ent.Name()
		if ent.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if !inBuild(t, file) {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			routers := routerParams(fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				callExpr, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := callExpr.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != http.MethodGet || len(callExpr.Args) < 2 {
					return true
				}
				recv, ok := sel.X.(*ast.Ident)
				if !ok || routers[recv.Name] == "" {
					return true
				}
				lit, ok := callExpr.Args[0].(*ast.BasicLit)
				if !ok {
					return true
				}
				path, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				if routers[recv.Name] == "group" {
					path = apiPrefix + path
				}
				for _, arg := range callExpr.Args[2:] {
					mwCall, ok := arg.(*ast.CallExpr)
					if !ok {
						continue
					}
					switch calleeName(mwCall.Fun) {
					case "cache", "cacheF":
						out[http.MethodGet+" "+path] = calleeName(mwCall.Fun) == "cacheF"
					}
				}
				return true
			})
		}
	}
	if len(out) == 0 {
		t.Fatal("no cached GET routes found; the declarations changed shape")
	}
	return out
}
