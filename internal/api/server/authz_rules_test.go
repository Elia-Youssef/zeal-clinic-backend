package server

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"clinic-api/internal/buildmode"
)

// The expected protection of every route is read from the declarations, not
// from the running router: the registrars in internal/api/routes are parsed,
// and the few routes registered elsewhere are listed in serverRules.

// Guard kinds, in the order a request meets them on a route.
const (
	guardJWT         = "jwt"            // bearer token checked by the auth middleware
	guardScope       = "scope"          // the listed scope
	guardAnyScope    = "any"            // at least one of the listed scopes
	guardAllScopes   = "all"            // every listed scope
	guardScopeOrSelf = "self"           // the scope, or :id is the caller's own record
	guardCritical    = "critical"       // cloud only: closed until a clinic has synced
	guardSyncSecret  = "sync-secret"    // X-Sync-Secret header, or the sync_secret query parameter
	guardSyncHeader  = "sync-header"    // X-Sync-Secret header only
	guardPublish     = "publish-header" // X-Publish-Secret header
	guardVersion     = "version"        // X-Sync-Version equal to the build version
)

type guard struct {
	Kind   string
	Scopes []string
	Self   string // "employee" or "user", for scope-or-self
}

// routeRule is the declared protection of one route.
type routeRule struct {
	Method string
	Path   string
	Guards []guard
	Build  string // "clinic" or "cloud" when the route exists on one build only
	Origin string // where the route is declared
}

func (r routeRule) key() string { return r.Method + " " + r.Path }

func (r routeRule) has(kind string) bool {
	return slices.ContainsFunc(r.Guards, func(g guard) bool { return g.Kind == kind })
}

// describe renders the guards in declaration order, as in the golden files.
func (r routeRule) describe() string {
	if len(r.Guards) == 0 {
		return "public"
	}
	parts := make([]string, 0, len(r.Guards))
	for _, g := range r.Guards {
		switch g.Kind {
		case guardScope:
			parts = append(parts, "scope("+g.Scopes[0]+")")
		case guardAnyScope:
			parts = append(parts, "any("+strings.Join(g.Scopes, ",")+")")
		case guardAllScopes:
			parts = append(parts, "all("+strings.Join(g.Scopes, ",")+")")
		case guardScopeOrSelf:
			parts = append(parts, "scope("+g.Scopes[0]+")|self("+g.Self+")")
		default:
			parts = append(parts, g.Kind)
		}
	}
	return strings.Join(parts, " ")
}

// apiPrefix is the prefix of every group server.go hands to the registrars.
const apiPrefix = "/api"

// publicGroups names the *echo.Group parameters that server.go mounts without
// the auth middleware. Every other group parameter is JWT-protected.
var publicGroups = map[string][]string{
	"SetupAuthRoutes": {"public"},
}

// serverRules are the routes registered outside internal/api/routes: the ones
// server.go mounts itself, and the machine endpoints of the sync and
// cloud-restore packages.
var serverRules = []routeRule{
	{Method: "GET", Path: "/health", Origin: "server.go"},
	{Method: "GET", Path: "/robots.txt", Origin: "server.go"},
	{Method: "GET", Path: "/*", Origin: "server.go (single-page app)"},
	{Method: "GET", Path: "/files/*", Origin: "server.go (generated files)", Guards: []guard{{Kind: guardJWT}}},
	{Method: "POST", Path: "/api/cloud-restore", Build: "clinic", Origin: "server.go",
		Guards: []guard{{Kind: guardJWT}, {Kind: guardScope, Scopes: []string{"cloud-restore:write"}}}},
	{Method: "POST", Path: "/api/cloud-restore", Build: "cloud", Origin: "cloudrestore/api.go (checked in the handler)",
		Guards: []guard{{Kind: guardSyncHeader}, {Kind: guardVersion}}},
	{Method: "GET", Path: "/api/sync/pull", Build: "cloud", Origin: "sync/api.go", Guards: syncGuards(true)},
	{Method: "POST", Path: "/api/sync/push", Build: "cloud", Origin: "sync/api.go", Guards: syncGuards(true)},
	{Method: "POST", Path: "/api/sync/ready", Build: "cloud", Origin: "sync/api.go", Guards: syncGuards(true)},
	{Method: "POST", Path: "/api/sync/failed", Build: "cloud", Origin: "sync/api.go", Guards: syncGuards(true)},
	{Method: "GET", Path: "/api/sync/events", Build: "cloud", Origin: "sync/api.go", Guards: syncGuards(true)},
	{Method: "GET", Path: "/api/sync/status", Build: "cloud", Origin: "sync/api.go", Guards: syncGuards(false)},
}

func syncGuards(versioned bool) []guard {
	g := []guard{{Kind: guardSyncSecret}}
	if versioned {
		g = append(g, guard{Kind: guardVersion})
	}
	return g
}

// expectedRules returns the declared rules of the routes on the running build.
func expectedRules(t *testing.T) []routeRule {
	t.Helper()
	var rules []routeRule
	for _, r := range append(parseRouteRules(t), serverRules...) {
		if r.Build == "" || r.Build == buildName() {
			rules = append(rules, r)
		}
	}
	return rules
}

// parseRouteRules reads every route registration in internal/api/routes.
func parseRouteRules(t *testing.T) []routeRule {
	t.Helper()
	dir := filepath.Join(packageDir, "..", "routes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read route declarations: %v", err)
	}
	fset := token.NewFileSet()
	var rules []routeRule
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
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			p := &ruleParser{t: t, fset: fset, fn: fn.Name.Name, routers: routerParams(fn)}
			if len(p.routers) == 0 {
				continue
			}
			p.walk(fn.Body.List, "")
			if n := countRouterCalls(fn, p.routers); n != len(p.rules) {
				t.Fatalf("%s: %d calls on its router parameters, %d understood as routes; extend the parser for the new form",
					fn.Name.Name, n, len(p.rules))
			}
			rules = append(rules, p.rules...)
		}
	}
	return rules
}

// inBuild evaluates a file's //go:build line for the running build.
func inBuild(t *testing.T, file *ast.File) bool {
	t.Helper()
	for _, group := range file.Comments {
		if group.Pos() > file.Package {
			break
		}
		for _, c := range group.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			expr, err := constraint.Parse(c.Text)
			if err != nil {
				t.Fatalf("build constraint %q: %v", c.Text, err)
			}
			return expr.Eval(func(tag string) bool {
				switch tag {
				case "cloud":
					return buildmode.Cloud
				case runtime.GOOS, runtime.GOARCH:
					return true
				}
				return false
			})
		}
	}
	return true
}

// routerParams maps a registrar's *echo.Group and *echo.Echo parameters to
// "group" or "echo".
func routerParams(fn *ast.FuncDecl) map[string]string {
	params := map[string]string{}
	for _, field := range fn.Type.Params.List {
		star, ok := field.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "echo" {
			continue
		}
		kind := map[string]string{"Group": "group", "Echo": "echo"}[sel.Sel.Name]
		if kind == "" {
			continue
		}
		for _, n := range field.Names {
			params[n.Name] = kind
		}
	}
	return params
}

// countRouterCalls counts every method call on a router parameter, wherever
// it sits in the function.
func countRouterCalls(fn *ast.FuncDecl, routers map[string]string) int {
	n := 0
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if recv, ok := sel.X.(*ast.Ident); ok && routers[recv.Name] != "" {
				n++
			}
		}
		return true
	})
	return n
}

type ruleParser struct {
	t       *testing.T
	fset    *token.FileSet
	fn      string
	routers map[string]string
	rules   []routeRule
}

func (p *ruleParser) fail(node ast.Node, format string, args ...any) {
	p.t.Helper()
	p.t.Fatalf("%s: "+format, append([]any{p.fset.Position(node.Pos())}, args...)...)
}

// walk collects the registrations in a statement list. build narrows them to
// one build behind a buildmode.Cloud check; any other condition (such as a
// configured secret) is taken as true, since the tests configure every secret.
func (p *ruleParser) walk(stmts []ast.Stmt, build string) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *ast.ExprStmt:
			if call, ok := s.X.(*ast.CallExpr); ok {
				p.register(call, build)
			}
		case *ast.BlockStmt:
			p.walk(s.List, build)
		case *ast.IfStmt:
			cond := p.buildCondition(s.Cond)
			p.walk(s.Body.List, p.narrow(s, build, cond))
			switch els := s.Else.(type) {
			case nil:
				if cond != "" && endsWithReturn(s.Body) {
					// if !buildmode.Cloud { return }: what follows runs on the other build.
					build = p.narrow(s, build, otherBuild(cond))
				}
			case *ast.BlockStmt:
				p.walk(els.List, p.narrow(s, build, otherBuild(cond)))
			default:
				p.fail(s, "else-if chains are not supported around route registrations")
			}
		}
	}
}

func (p *ruleParser) narrow(node ast.Node, build, cond string) string {
	switch {
	case cond == "" || cond == build:
		return build
	case build == "":
		return cond
	}
	p.fail(node, "registrations under contradicting build conditions")
	return ""
}

func otherBuild(b string) string {
	switch b {
	case "cloud":
		return "clinic"
	case "clinic":
		return "cloud"
	}
	return ""
}

func endsWithReturn(body *ast.BlockStmt) bool {
	if len(body.List) == 0 {
		return false
	}
	_, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
	return ok
}

// buildCondition reports the build an if-condition selects: "cloud" for
// buildmode.Cloud, "clinic" for !buildmode.Cloud, "" when it does not look at
// the build.
func (p *ruleParser) buildCondition(cond ast.Expr) string {
	e := cond
	negated := false
	if u, ok := e.(*ast.UnaryExpr); ok && u.Op == token.NOT {
		negated, e = true, u.X
	}
	if isBuildmodeCloud(e) {
		if negated {
			return "clinic"
		}
		return "cloud"
	}
	mentions := false
	ast.Inspect(cond, func(n ast.Node) bool {
		if x, ok := n.(ast.Expr); ok && isBuildmodeCloud(x) {
			mentions = true
		}
		return !mentions
	})
	if mentions {
		p.fail(cond, "only buildmode.Cloud and !buildmode.Cloud are supported as build conditions")
	}
	return ""
}

func isBuildmodeCloud(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Cloud" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "buildmode"
}

var routeMethods = map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}

// register records a call such as api.GET("/rooms", handler, scope("rooms:read")).
func (p *ruleParser) register(call *ast.CallExpr, build string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || p.routers[recv.Name] == "" {
		return
	}
	method := sel.Sel.Name
	if !routeMethods[method] {
		p.fail(call, "unsupported router call %s.%s", recv.Name, method)
	}
	if len(call.Args) < 2 {
		p.fail(call, "route registration without a handler")
	}
	path := p.stringArgs(call, call.Args[:1], 1)[0]
	var guards []guard
	if p.routers[recv.Name] == "group" {
		path = apiPrefix + path
		if !slices.Contains(publicGroups[p.fn], recv.Name) {
			guards = append(guards, guard{Kind: guardJWT})
		}
	}
	for _, arg := range call.Args[2:] {
		if g, ok := p.guard(arg); ok {
			guards = append(guards, g)
		}
	}
	pos := p.fset.Position(call.Pos())
	p.rules = append(p.rules, routeRule{
		Method: method,
		Path:   path,
		Guards: guards,
		Build:  build,
		Origin: "routes/" + filepath.Base(pos.Filename) + ":" + strconv.Itoa(pos.Line),
	})
}

// guard classifies one route middleware. Middleware that does not decide
// access (response cache, login rate limit) returns false; anything unknown
// fails the test, so a new kind of guard cannot go unchecked.
func (p *ruleParser) guard(arg ast.Expr) (guard, bool) {
	switch a := arg.(type) {
	case *ast.Ident:
		if a.Name == "criticalSync" {
			return guard{Kind: guardCritical}, true
		}
	case *ast.CallExpr:
		switch calleeName(a.Fun) {
		case "scope", "RequireScope":
			return guard{Kind: guardScope, Scopes: p.stringArgs(a, a.Args, 1)}, true
		case "scopeAny", "RequireAnyScope":
			return guard{Kind: guardAnyScope, Scopes: p.stringArgs(a, a.Args, -1)}, true
		case "scopeAll", "RequireAllScopes":
			return guard{Kind: guardAllScopes, Scopes: p.stringArgs(a, a.Args, -1)}, true
		case "scopeOrSelf", "RequireScopeOrSelf":
			if len(a.Args) != 2 {
				p.fail(a, "scope-or-self takes a scope and a self lookup")
			}
			self := map[string]string{
				"selfEmployee": "employee", "SelfEmployeeID": "employee",
				"selfUser": "user", "SelfUserID": "user",
			}[calleeName(a.Args[1])]
			if self == "" {
				p.fail(a, "unknown self lookup")
			}
			return guard{Kind: guardScopeOrSelf, Scopes: p.stringArgs(a, a.Args[:1], 1), Self: self}, true
		case "RequireSyncSecret":
			return guard{Kind: guardSyncHeader}, true
		case "RequirePublishSecret":
			return guard{Kind: guardPublish}, true
		case "cache", "cacheF", "CacheMiddleware", "CacheMiddlewareForce", "loginRateLimiter":
			return guard{}, false
		}
	}
	p.fail(arg, "unknown route middleware; classify it in this parser")
	return guard{}, false
}

func calleeName(e ast.Expr) string {
	switch f := e.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// stringArgs unquotes string-literal arguments; want < 0 accepts any
// non-zero count.
func (p *ruleParser) stringArgs(at ast.Node, args []ast.Expr, want int) []string {
	if len(args) == 0 || want >= 0 && len(args) != want {
		p.fail(at, "expected %d string arguments, got %d", want, len(args))
	}
	out := make([]string, 0, len(args))
	for _, a := range args {
		lit, ok := a.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			p.fail(a, "expected a string literal")
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			p.fail(a, "unquote: %v", err)
		}
		out = append(out, s)
	}
	return out
}
