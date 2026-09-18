package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"clinic-api/internal/buildmode"
	"clinic-api/internal/database/store"
)

// TestContract builds the scenario and records every route's behavior in the
// contract goldens: every GET, success and typical errors of every write, PDF
// downloads, the envelope, headers and the routes outside /api.
func TestContract(t *testing.T) {
	if nearMidnight(time.Now()) {
		t.Skip("the contract cases are read relative to the day; run them away from midnight (00:15 to 23:45 clinic time, not within 2 minutes of midnight UTC)")
	}
	checkDSTVectors(t)
	r := newContractRun(t)
	s := &contractScenario{}
	r.cache.probes = func() []string { return r.cacheProbes(s) }

	start := time.Now()
	buildContractScenario(r, s)
	r.phase = phaseCases
	r.getSweep(s)
	r.todayAppointment(s)
	r.specificErrors(s)
	r.writeErrorSweep()
	r.authCases(s)
	r.serverCases(s)
	r.pdfCases(s)
	r.buildCases(s)
	t.Logf("%s build: %d files, %d requests in %s", buildName(), len(r.files), r.seq, time.Since(start).Round(time.Millisecond))

	r.cache.selfTest(r)

	r.checkCoverage(noSuccessCase, noClientErrorCase)
	r.finish()
	r.cache.finish(r)
}

// nearMidnight reports whether a run starting at now could see the clinic's
// day or the UTC day change before it ends.
func nearMidnight(now time.Time) bool {
	clinic := now.In(store.ClinicLocation())
	utc := now.UTC()
	return (clinic.Hour() == 23 && clinic.Minute() >= 45) || (clinic.Hour() == 0 && clinic.Minute() < 15) ||
		(utc.Hour() == 23 && utc.Minute() >= 58) || (utc.Hour() == 0 && utc.Minute() < 2)
}

// Routes whose success or client error can't be produced in a test process.
var noSuccessCase = map[string]string{
	"POST /api/update/start":  "no newer version is published in a fresh database",
	"POST /api/cloud-restore": "needs a reachable peer (clinic) or a database dump (cloud)",
	"POST /api/versions":      "publishing a version would offer an update to install",
	"POST /api/sync/ready":    "needs the session token of the open sync stream",
	"POST /api/sync/failed":   "would fail the open sync session",
}

var noClientErrorCase = map[string]string{
	"POST /api/auth/logout":           "always answers 200",
	"POST /api/notifications/test":    "takes no input",
	"PUT /api/notifications/read-all": "takes no input",
}

type callOpt func(*contractCall)

func orderedCase(c *contractCall) { c.orderedCase = true }
func keepOrder(paths ...string) callOpt {
	return func(c *contractCall) { c.keepOrder = append(c.keepOrder, paths...) }
}
func utcDays(c *contractCall)           { c.utcDays = true }
func asCaller(a *contractActor) callOpt { return func(c *contractCall) { c.asCaller = a } }
func caseNamed(name string) callOpt     { return func(c *contractCall) { c.name = name } }
func onBuild(b string) callOpt          { return func(c *contractCall) { c.build = b } }

// getCalls builds GET calls; paths holding an id the scenario hasn't made yet
// are left out, so the same list serves the cache check while the scenario
// is still being built.
type getCalls struct{ list []contractCall }

func (g *getCalls) add(path string, opts ...callOpt) {
	if strings.Contains(path, "\x00") {
		return
	}
	c := contractCall{method: http.MethodGet, path: path}
	for _, o := range opts {
		o(&c)
	}
	g.list = append(g.list, c)
}

// scenarioID stands for a scenario id in a path.
func scenarioID(v string) string {
	if v == "" {
		return "\x00"
	}
	return v
}

// dateWindows are the date ranges of the report and analytics cases.
type dateWindows struct {
	nowFrom, nowTo   string // bare dates around the run's day (server timestamps)
	rfcFrom, rfcTo   string // the same days as UTC instants of clinic midnights
	weekFrom, weekTo string // a week each side, for the appointments near today
	fixFrom, fixTo   string // the fixed appointments of 2025
}

func (r *contractRun) dateWindows() dateWindows {
	midnight := func(offset int) string {
		d := r.norm.today.AddDate(0, 0, offset)
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, store.ClinicLocation()).UTC().Format(time.RFC3339)
	}
	return dateWindows{
		nowFrom: r.day(-1), nowTo: r.day(1),
		rfcFrom: midnight(-1), rfcTo: midnight(2),
		weekFrom: r.day(-7), weekTo: r.day(7),
		fixFrom: "2025-03-24", fixTo: "2025-11-02",
	}
}

// withQuery appends a query string. Values are written as they are, so dates stay
// readable in the case names; only spaces are escaped.
func withQuery(path string, kv ...string) string {
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, kv[i]+"="+strings.ReplaceAll(kv[i+1], " ", "+"))
	}
	return path + "?" + strings.Join(parts, "&")
}

// getList is every GET case of the main sweep, in file order.
func (r *contractRun) getList(s *contractScenario) []contractCall {
	w := r.dateWindows()
	g := &getCalls{}
	now := func(p string, kv ...string) string {
		return withQuery(p, append([]string{"from", w.nowFrom, "to", w.nowTo}, kv...)...)
	}
	fixed := func(p string, kv ...string) string {
		return withQuery(p, append([]string{"from", w.fixFrom, "to", w.fixTo}, kv...)...)
	}
	week := func(p string, kv ...string) string {
		return withQuery(p, append([]string{"from", w.weekFrom, "to", w.weekTo}, kv...)...)
	}

	g.add("/api/allergies?sort=name&order=asc", orderedCase)
	g.add("/api/allergies?filter=latex")
	g.add("/api/allergies/dropdown", orderedCase)

	g.add(now("/api/analytics/money"))
	g.add(withQuery("/api/analytics/money", "from", w.rfcFrom, "to", w.rfcTo), caseNamed("GET /api/analytics/money between clinic midnights"))
	g.add("/api/analytics/money", caseNamed("GET /api/analytics/money for the last 30 days"))
	g.add(now("/api/analytics/patients"))
	g.add("/api/analytics/patients", caseNamed("GET /api/analytics/patients for the last 30 days"))
	g.add(fixed("/api/analytics/operations"))
	g.add(week("/api/analytics/operations"))
	g.add("/api/analytics/inventory")
	bands := keepOrder(".Data.ageBands", ".Data.gender", ".Data.topCities")
	g.add("/api/analytics/demographics", bands)
	g.add("/api/analytics/demographics?cityLimit=1", bands)
	g.add(now("/api/analytics/referral-sources"))
	g.add(fixed("/api/analytics/staff-performance"))
	g.add(week("/api/analytics/staff-performance"))
	g.add(now("/api/analytics/procedures/top"))
	g.add(now("/api/analytics/procedures/top", "by", "revenue", "limit", "1"))
	g.add(now("/api/analytics/products/top"))
	g.add(fixed("/api/analytics/appointments/distribution"))
	g.add(fixed("/api/analytics/rooms/utilization"))
	g.add("/api/analytics/transactions/recent?limit=100")
	g.add(now("/api/analytics/series", "metric", "revenue"), utcDays, orderedCase)
	g.add(now("/api/analytics/series", "metric", "expenses", "groupBy", "day"), utcDays, orderedCase)
	g.add(now("/api/analytics/series", "metric", "new-patients"), utcDays, orderedCase)
	g.add(now("/api/analytics/series", "metric", "procedures-completed"), utcDays, orderedCase)
	g.add(fixed("/api/analytics/series", "metric", "appointments", "groupBy", "month"), orderedCase)
	g.add(fixed("/api/analytics/series", "metric", "appointments", "groupBy", "week"), orderedCase)
	g.add(fixed("/api/analytics/series", "metric", "appointments"), orderedCase)
	g.add(now("/api/analytics/series"))
	g.add(now("/api/analytics/series", "metric", "profit"))
	g.add(now("/api/analytics/series", "metric", "revenue", "groupBy", "year"))
	g.add("/api/analytics/money?from=yesterday")
	g.add("/api/analytics/series?metric=revenue&from=2025-05")

	g.add("/api/appointments?date=2025-03-29")
	g.add("/api/appointments?date=2025-03-31")
	g.add("/api/appointments?date=2025-10-26")
	g.add("/api/appointments?date=2025-10-27")
	g.add(withQuery("/api/appointments", "date", r.day(2)))
	g.add("/api/appointments?date=2025-03-31&sort=startTime&order=desc", orderedCase)
	g.add("/api/appointments")
	g.add("/api/appointments/count-per-room?date=2025-03-31", keepOrder(".Data.rooms"))
	g.add("/api/appointments/count-per-room?date=2025-10-27", keepOrder(".Data.rooms"))
	g.add("/api/appointments/count-per-room")
	g.add("/api/appointments/count-per-room?date=31-03-2025")
	g.add("/api/patients/" + scenarioID(s.ada) + "/appointments")
	g.add("/api/patients/"+scenarioID(s.ben)+"/appointments?sort=startTime&order=asc", orderedCase)
	g.add("/api/employees/" + scenarioID(s.nora) + "/appointments?date=2025-03-31")
	g.add("/api/employees/" + scenarioID(s.nora) + "/appointments?date=2025-10-27")
	g.add("/api/employees/"+scenarioID(s.nora)+"/appointments?date=2025-03-31", asCaller(s.nurse), caseNamed("nurse reads her own appointments"))

	g.add("/api/audit-log")
	g.add("/api/audit-log?entityType=patients")
	g.add("/api/audit-log?action=update&userId=" + scenarioID(s.adminUser))
	g.add("/api/audit-log?filter=serum")

	g.add("/api/auth/me")
	g.add("/api/auth/me", asCaller(s.admin), caseNamed("admin reads /api/auth/me"))
	g.add("/api/auth/me", asCaller(s.staff), caseNamed("staff reads /api/auth/me"))
	g.add("/api/auth/me", asCaller(s.nurse), caseNamed("nurse reads /api/auth/me"))
	g.add("/api/auth/verify")

	for _, typ := range []string{"patient", "employee", "self", "supplier", "expense"} {
		g.add("/api/balances/"+typ, orderedCase)
	}
	g.add("/api/balances/unknown")
	g.add("/api/balances/patient?filter=Ben")
	g.add("/api/balances/patient/" + scenarioID(s.ada))
	g.add("/api/balances/patient/" + scenarioID(s.cleo))
	g.add("/api/balances/employee/" + scenarioID(s.nora))
	g.add("/api/balances/supplier/" + scenarioID(s.acme))
	g.add("/api/balances/expense/" + scenarioID(s.rent))
	g.add("/api/balances/self/self")
	g.add("/api/balances/patient/" + unknownID)
	g.add("/api/balances/employee/"+scenarioID(s.nora), asCaller(s.nurse), caseNamed("nurse reads her own balance"))

	g.add("/api/patients/" + scenarioID(s.ada) + "/invoices")
	g.add("/api/patients/" + scenarioID(s.ben) + "/invoices")
	g.add("/api/patients/" + scenarioID(s.ada) + "/payments")
	g.add("/api/patients/"+scenarioID(s.ben)+"/payments?sort=amount&order=asc", orderedCase)

	g.add("/api/countries?limit=3", orderedCase)
	g.add("/api/countries?filter=leban")
	g.add("/api/countries/dropdown?filter=united", orderedCase)
	g.add("/api/currencies", orderedCase)
	g.add("/api/currencies/dropdown", orderedCase)

	g.add("/api/discounts")
	g.add("/api/discounts?type=offer&sort=name&order=asc", orderedCase)
	g.add("/api/discounts?type=gift")
	g.add("/api/discounts/" + scenarioID(s.spring))
	g.add("/api/discounts/" + scenarioID(s.giftID))
	g.add("/api/discounts/" + unknownID)
	g.add("/api/discounts/" + scenarioID(s.spring) + "/invoices")
	g.add("/api/discounts/" + scenarioID(s.giftID) + "/invoices")

	g.add("/api/employees/" + scenarioID(s.nora) + "/payments")
	g.add("/api/employees/"+scenarioID(s.nora)+"/payments", asCaller(s.nurse), caseNamed("nurse reads her own payments"))
	g.add("/api/employees/" + scenarioID(s.nora) + "/salaries")
	g.add("/api/employees/" + scenarioID(s.nora) + "/prepared-salaries")
	g.add("/api/employees/" + scenarioID(s.sam) + "/prepared-salaries")
	g.add("/api/employees/"+scenarioID(s.nora)+"/salaries", asCaller(s.nurse), caseNamed("nurse reads her own salaries"))

	g.add("/api/employees?sort=firstName&order=asc", orderedCase)
	g.add("/api/employees?filter=Sample")
	g.add("/api/employees/dropdown", orderedCase)
	g.add("/api/employees/" + scenarioID(s.nora))
	g.add("/api/employees/" + unknownID)
	g.add("/api/employees/"+scenarioID(s.nora), asCaller(s.nurse), caseNamed("nurse reads her own employee record"))

	g.add("/api/expenses/" + scenarioID(s.rent) + "/payments")
	g.add("/api/expenses/" + scenarioID(s.utilities) + "/payments")
	g.add("/api/expenses?sort=name&order=asc", orderedCase)
	g.add("/api/expenses/dropdown", orderedCase)
	g.add("/api/expenses/" + scenarioID(s.rent))
	g.add("/api/expenses/" + unknownID)

	days := keepOrder(".Data.days")
	g.add("/api/employees/"+scenarioID(s.nora)+"/schedule?date=2025-03-31", days)
	g.add("/api/employees/"+scenarioID(s.nora)+"/schedule?date=2025-04-09", days)
	g.add("/api/employees/"+scenarioID(s.nora)+"/schedule?date=2025-10-27", days)
	g.add("/api/employees/"+scenarioID(s.sam)+"/schedule?date=2025-03-05", days)
	g.add("/api/employees/"+scenarioID(s.nora)+"/schedule?date=2025-03-31", days, asCaller(s.nurse), caseNamed("nurse reads her own schedule"))
	g.add("/api/employees/"+scenarioID(s.nora)+"/working-hours?from=2025-03-24&to=2025-04-06", days)
	g.add("/api/employees/"+scenarioID(s.nora)+"/working-hours?from=2025-10-20&to=2025-11-02", days)
	g.add("/api/employees/" + scenarioID(s.nora) + "/working-hours?from=2025-03-24")
	g.add("/api/holidays?sort=startDate&order=asc", orderedCase)

	g.add("/api/invoices")
	g.add("/api/invoices?type=patient&sort=invoiceNumber&order=asc", orderedCase)
	g.add("/api/invoices?type=supplier")
	g.add("/api/invoices?filter=Ada")
	g.add(now("/api/invoices", "type", "patient"))
	g.add("/api/invoices/" + scenarioID(s.invoice1))
	g.add("/api/invoices/" + scenarioID(s.supplierInvoice))
	g.add("/api/invoices/" + unknownID)

	g.add("/api/lebanon-cities?limit=3", orderedCase)
	g.add("/api/lebanon-cities?filter=jounieh")
	g.add("/api/lebanon-cities/dropdown?filter=beirut", orderedCase)

	g.add("/api/medicines?sort=name&order=asc", orderedCase)
	g.add("/api/medicines/dropdown", orderedCase)

	g.add("/api/notifications")
	g.add("/api/notifications/unread-count")
	g.add("/api/notifications", asCaller(s.nurse), caseNamed("nurse reads her notifications"))
	g.add("/api/notifications/unread-count", asCaller(s.nurse), caseNamed("nurse reads her unread count"))

	g.add("/api/patients/" + scenarioID(s.ada) + "/allergies")
	g.add("/api/patients/" + scenarioID(s.ben) + "/allergies")
	g.add("/api/patients/" + scenarioID(s.ada) + "/medicines")

	g.add("/api/patients?sort=firstName&order=asc", orderedCase)
	g.add("/api/patients?sort=firstName&order=asc&limit=1&offset=1", orderedCase)
	g.add("/api/patients?filter=ada+example")
	g.add("/api/patients/dropdown", orderedCase)
	g.add("/api/patients/dropdown?filter=71123456")
	g.add("/api/patients/" + scenarioID(s.ada))
	g.add("/api/patients/" + scenarioID(s.cleo))
	g.add("/api/patients/" + unknownID)

	g.add("/api/patients/" + scenarioID(s.ada) + "/prescriptions")
	g.add("/api/patients/" + scenarioID(s.cleo) + "/prescriptions")

	g.add("/api/procedures/" + scenarioID(s.peel) + "/allergy-conflicts")
	g.add("/api/procedure-categories?filter=Test&sort=name&order=asc", orderedCase)
	g.add("/api/procedure-categories/dropdown?filter=Test", orderedCase)
	g.add("/api/procedure-types?sort=name&order=asc", orderedCase)
	g.add("/api/procedure-types/dropdown", orderedCase)

	g.add("/api/procedures?filter=Test&sort=name&order=asc", orderedCase)
	g.add("/api/procedures?filter=Botox+Full&sort=price&order=desc&limit=2", orderedCase)
	g.add("/api/procedures/dropdown?filter=Test", orderedCase)
	g.add("/api/procedures/" + scenarioID(s.peel))
	g.add("/api/procedures/b76c82c3-e031-45e3-b6c3-e51e7560a997")
	g.add("/api/procedures/" + unknownID)
	g.add("/api/procedures/" + scenarioID(s.peel) + "/prices")
	g.add("/api/procedures/" + scenarioID(s.peel) + "/appointments")
	g.add("/api/procedures/"+scenarioID(s.laser)+"/appointments?sort=startTime&order=asc", orderedCase)

	g.add("/api/products/" + scenarioID(s.cream) + "/allergy-conflicts")
	g.add("/api/product-categories?sort=name&order=asc", orderedCase)
	g.add("/api/product-categories/dropdown", orderedCase)
	g.add("/api/products?sort=name&order=asc", orderedCase)
	g.add("/api/products/dropdown", orderedCase)
	g.add("/api/products/" + scenarioID(s.cream))
	g.add("/api/products/" + scenarioID(s.serum))
	g.add("/api/products/" + unknownID)
	g.add("/api/products/" + scenarioID(s.cream) + "/invoices")
	g.add("/api/products/" + scenarioID(s.serum) + "/invoices")
	g.add("/api/products/" + scenarioID(s.cream) + "/prices")
	g.add("/api/products/"+scenarioID(s.cream)+"/invoices", asCaller(s.nurse), caseNamed("nurse reads a product's invoices"))

	g.add(now("/api/reports/revenue"))
	g.add(withQuery("/api/reports/revenue", "from", w.rfcFrom, "to", w.rfcTo), caseNamed("GET /api/reports/revenue between clinic midnights"))
	g.add("/api/reports/revenue", caseNamed("GET /api/reports/revenue for the last 30 days"))
	for _, kind := range []string{"procedures", "products", "other", "discounts", "all"} {
		g.add(now("/api/reports/revenue", "itemKind", kind))
	}
	for _, level := range []string{"kind", "procedure-type", "procedure-category", "product-category", "procedure", "product", "other", "discount"} {
		g.add(now("/api/reports/revenue", "level", level))
	}
	g.add(now("/api/reports/revenue", "itemKind", "procedures", "typeId", scenarioID(s.aesthetic)))
	g.add(now("/api/reports/revenue", "itemKind", "procedures", "categoryId", scenarioID(s.skinCare)))
	g.add(now("/api/reports/revenue", "itemKind", "products", "categoryId", scenarioID(s.skincare)))
	g.add(now("/api/reports/revenue", "level", "bogus"))
	g.add(now("/api/reports/expenses"))
	g.add("/api/reports/expenses", caseNamed("GET /api/reports/expenses for the last 30 days"))
	g.add("/api/reports/revenue?from=2025&to=2025-05-31")
	g.add("/api/reports/expenses?to=bogus")

	g.add("/api/roles", orderedCase)
	g.add("/api/roles/dropdown", orderedCase)
	g.add("/api/roles/nurse")
	g.add("/api/roles/super-admin")
	g.add("/api/roles/missing-role")

	g.add("/api/rooms?sort=name&order=asc", orderedCase)
	g.add("/api/rooms?filter=Test")
	g.add("/api/rooms/dropdown", orderedCase)

	g.add("/api/search?q=Example")
	g.add("/api/search?q=test+serum")
	g.add("/api/search?q=")
	g.add("/api/search?q=Example", asCaller(s.nurse), caseNamed("nurse searches"))
	g.add("/api/search?q=Example", asCaller(s.staff), caseNamed("staff searches"))

	g.add("/api/suppliers/" + scenarioID(s.acme) + "/invoices")
	g.add("/api/suppliers/" + scenarioID(s.globex) + "/invoices")
	g.add("/api/suppliers/" + scenarioID(s.acme) + "/payments")
	g.add("/api/suppliers?sort=name&order=asc", orderedCase)
	g.add("/api/suppliers/dropdown", orderedCase)
	g.add("/api/suppliers/" + scenarioID(s.acme))
	g.add("/api/suppliers/" + unknownID)

	g.add("/api/update/status")

	g.add("/api/users?sort=username&order=asc", orderedCase)
	g.add("/api/users?filter=example")
	g.add("/api/users/" + scenarioID(s.adminUser))
	g.add("/api/users/" + seedSuperAdmin)
	g.add("/api/users/" + unknownID)
	g.add("/api/users/"+scenarioID(s.noraUser), asCaller(s.nurse), caseNamed("nurse reads her own user"))
	g.add("/api/users/" + scenarioID(s.adminUser) + "/actions")
	g.add("/api/users/" + scenarioID(s.samUser) + "/actions")
	g.add("/api/users/" + seedSuperAdmin + "/actions")
	return g.list
}

// cacheProbes are the sweep's GETs the response cache can serve, read as the
// super-admin.
func (r *contractRun) cacheProbes(s *contractScenario) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range r.getList(s) {
		if c.asCaller != nil || seen[c.path] || !r.cache.cacheable(r, c.path) {
			continue
		}
		seen[c.path] = true
		out = append(out, c.path)
	}
	return out
}

func (r *contractRun) getSweep(s *contractScenario) {
	for _, c := range r.getList(s) {
		r.run(c)
	}
	g := &getCalls{}
	g.add("/api/server-info", onBuild("clinic"), caseNamed("server info on the clinic"))
	g.add("/api/server-info", onBuild("cloud"), caseNamed("server info on the cloud"))
	for _, c := range g.list {
		if c.build == "" || c.build == buildName() {
			// The clinic reports this machine's address; name it before recording.
			rep := r.send(c)
			var info struct {
				Host string `json:"host"`
			}
			rep.data(r.t, &info)
			if buildName() == "clinic" {
				r.norm.host = info.Host
			}
		}
		r.run(c)
	}
	// The cloud reports a clinic as connected: the sync stream the test holds open.
	r.run(contractCall{name: "the event stream on the clinic", path: "/api/events", stream: 3, build: "clinic"})
	r.run(contractCall{name: "the event stream on the cloud", path: "/api/events", stream: 3, build: "cloud"})
}

// todayAppointment books an appointment on the run's own day, after the
// analytics that count upcoming appointments from the current time.
func (r *contractRun) todayAppointment(s *contractScenario) {
	r.phase = phaseScenario
	r.create("book today", "/api/appointments", jsonObject{"patientId": s.cleo, "roomId": seedRoom3,
		"startTime": r.day(0) + "T07:00:00.000Z", "endTime": r.day(0) + "T07:45:00.000Z",
		"appointmentProcedures": []jsonObject{{"procedureId": s.peel, "assignedToId": s.nora}}})
	r.phase = phaseCases
	r.run(contractCall{path: "/api/analytics/appointments/recent-today", orderedCase: true})
	r.run(contractCall{path: "/api/analytics/appointments/recent-today?limit=1", orderedCase: true})
	r.run(contractCall{path: withQuery("/api/appointments", "date", r.day(0))})
}

// specificErrors are the typical client errors that need scenario data: 409s
// for dependencies, uniqueness, stock and room conflicts, validation messages,
// and the handler-level 403s.
func (r *contractRun) specificErrors(s *contractScenario) {
	e := func(name, method, path string, body any, opts ...callOpt) {
		c := contractCall{name: name, method: method, path: path, body: body}
		for _, o := range opts {
			o(&c)
		}
		r.run(c)
	}
	post, put, del := http.MethodPost, http.MethodPut, http.MethodDelete

	e("delete an allergy in use", del, "/api/allergies/"+s.latex, nil)
	e("delete a medicine in use", del, "/api/medicines/"+s.amoxi, nil)
	e("delete a room in use", del, "/api/rooms/"+seedRoom1, nil)
	e("create a room of an unknown type", post, "/api/rooms", jsonObject{"name": "Test Garage", "type": "Garage"})
	e("delete the currency in use", del, "/api/currencies/USD", nil)
	e("create a currency without a name", post, "/api/currencies", jsonObject{"code": "CHF"})
	e("create a duplicate procedure type", post, "/api/procedure-types", jsonObject{"name": "clinic procedure"})
	e("rename a procedure type to a taken name", put, "/api/procedure-types/"+s.aesthetic, jsonObject{"name": "Hospital Surgery"})
	e("delete a procedure type in use", del, "/api/procedure-types/"+s.aesthetic, nil)
	e("create a duplicate procedure category", post, "/api/procedure-categories", jsonObject{"name": "test peels"})
	e("make a procedure category its own parent", put, "/api/procedure-categories/"+s.peels, jsonObject{"parentId": s.peels})
	e("delete a procedure category in use", del, "/api/procedure-categories/"+s.peels, nil)
	e("delete a parent procedure category", del, "/api/procedure-categories/"+s.skinCare, nil)
	e("delete a procedure in use", del, "/api/procedures/"+s.peel, nil)
	e("add a procedure allergy conflict twice", post, "/api/procedures/"+s.peel+"/allergy-conflicts", jsonObject{"allergyId": s.latex})
	e("create a duplicate product category", post, "/api/product-categories", jsonObject{"name": "Test Serums"})
	e("delete a product category in use", del, "/api/product-categories/"+s.serums, nil)
	e("delete a product in use", del, "/api/products/"+s.cream, nil)
	e("create a product with a negative price", post, "/api/products", jsonObject{"name": "Test Bad Price", "unitPrice": -1})
	e("add a product allergy conflict twice", post, "/api/products/"+s.cream+"/allergy-conflicts", jsonObject{"allergyId": s.pollen})
	e("delete a supplier in use", del, "/api/suppliers/"+s.acme, nil)
	e("create a supplier with a bad phone", post, "/api/suppliers", jsonObject{"name": "Test Bad Phone", "contact": "call me"})
	e("create a supplier with a bad email", post, "/api/suppliers", jsonObject{"name": "Test Bad Email", "email": "ok@example.test, nope"})
	e("delete an expense with payments", del, "/api/expenses/"+s.rent, nil)
	e("delete an offer used by an invoice", del, "/api/discounts/"+s.spring, nil)
	e("create a gift card directly", post, "/api/discounts", jsonObject{"name": "Test Gift", "discountType": "gift", "valueType": "fixed", "value": 5, "code": "X"})
	e("create an offer over 100 percent", post, "/api/discounts", jsonObject{"name": "Test Too Much", "discountType": "offer", "valueType": "percentage", "value": 150})
	e("create an offer ending before it starts", post, "/api/discounts", jsonObject{"name": "Test Backwards", "discountType": "offer",
		"valueType": "fixed", "value": 5, "startDate": "2025-05-02", "endDate": "2025-05-01"})
	e("rename a gift card and change its value", put, "/api/discounts/"+s.giftID, jsonObject{"name": "Renamed Gift", "value": 999})

	e("delete a patient with appointments", del, "/api/patients/"+s.ada, nil)
	e("create a patient of an unknown gender", post, "/api/patients", jsonObject{"firstName": "Test", "lastName": "Gender", "gender": "Other", "contact": "70999000"})
	e("create a patient born in the future", post, "/api/patients", jsonObject{"firstName": "Test", "lastName": "Future", "gender": "Male",
		"contact": "70999001", "dateOfBirth": r.day(1)})
	e("create a patient with a bad phone", post, "/api/patients", jsonObject{"firstName": "Test", "lastName": "Phone", "gender": "Male", "contact": "12"})
	e("update a patient with a bad email", put, "/api/patients/"+s.ben, jsonObject{"email": "not-an-email"})
	e("add a patient allergy twice", post, "/api/patients/"+s.ada+"/allergies", jsonObject{"allergyId": s.latex})
	e("add an unknown medicine to a patient", post, "/api/patients/"+s.ben+"/medicines", jsonObject{"medicineId": unknownID})
	e("create a prescription without a start date", post, "/api/prescriptions", jsonObject{"patientId": s.ben, "prescribedById": s.otto})
	e("create a prescription with a bad end date", post, "/api/prescriptions", jsonObject{"patientId": s.ben, "prescribedById": s.otto,
		"startDate": "2025-04-01", "endDate": "01/05/2025"})

	e("delete an employee with records", del, "/api/employees/"+s.nora, nil)
	e("create an employee with an unknown employment type", post, "/api/employees", jsonObject{"firstName": "Test", "lastName": "Contract",
		"role": "Nurse", "contact": "70999002", "employmentType": "Contract"})
	e("create an employee with a taken username", post, "/api/employees", jsonObject{"firstName": "Test", "lastName": "Taken",
		"role": "Nurse", "contact": "70999003", "employmentType": "Full-time", "username": "nora.example", "userRole": "nurse"})
	e("create an employee account with the super-admin role", post, "/api/employees", jsonObject{"firstName": "Test", "lastName": "Super",
		"role": "Nurse", "contact": "70999004", "employmentType": "Full-time", "username": "test.super", "userRole": "super-admin"})
	e("create a user with the super-admin role", post, "/api/users", jsonObject{"username": "second.super", "displayName": "Second", "role": "super-admin"})
	e("create a user with a short username", post, "/api/users", jsonObject{"username": "ab", "displayName": "Short", "role": "staff"})
	e("create a user with a taken username", post, "/api/users", jsonObject{"username": "admin.example", "displayName": "Again", "role": "staff"})
	e("update the super-admin user", put, "/api/users/"+seedSuperAdmin, jsonObject{"displayName": "Renamed"})
	e("give a user the super-admin role", put, "/api/users/"+s.unlinkedUser, jsonObject{"role": "super-admin"})
	e("demote the last admin", put, "/api/users/"+s.adminUser, jsonObject{"role": "staff"})
	e("admin changes their own role", put, "/api/users/"+s.adminUser, jsonObject{"role": "nurse"}, asCaller(s.admin))
	e("admin deactivates their own account", put, "/api/users/"+s.adminUser, jsonObject{"isActive": false}, asCaller(s.admin))
	e("update the super-admin role", put, "/api/roles/super-admin", jsonObject{"label": "Renamed"})
	e("strip the admin role of user management", put, "/api/roles/admin", jsonObject{"scopes": []string{"patients:read"}})
	e("grant an unknown permission", put, "/api/roles/nurse", jsonObject{"scopes": []string{"patients:read", "bogus:read"}})

	e("book over an existing appointment", post, "/api/appointments", jsonObject{"patientId": s.cleo, "roomId": seedRoom1,
		"startTime": clinicUTC(r.t, "2025-03-29", "10:30"), "endTime": clinicUTC(r.t, "2025-03-29", "11:30")})
	e("book with a bad start time", post, "/api/appointments", jsonObject{"patientId": s.cleo, "roomId": seedRoom1,
		"startTime": "tomorrow", "endTime": clinicUTC(r.t, "2025-03-29", "11:30")})
	e("book with an unknown status", post, "/api/appointments", jsonObject{"patientId": s.cleo, "roomId": seedRoom1,
		"startTime": clinicUTC(r.t, "2025-05-01", "10:00"), "endTime": clinicUTC(r.t, "2025-05-01", "11:00"), "status": "Maybe"})
	e("book for an unknown patient", post, "/api/appointments", jsonObject{"patientId": unknownID, "roomId": seedRoom1,
		"startTime": clinicUTC(r.t, "2025-05-01", "10:00"), "endTime": clinicUTC(r.t, "2025-05-01", "11:00")})
	e("move an appointment onto a booked slot", put, "/api/appointments/"+s.apptHoliday, jsonObject{"roomId": seedRoom1,
		"startTime": clinicUTC(r.t, "2025-03-29", "10:15"), "endTime": clinicUTC(r.t, "2025-03-29", "10:45")})
	e("reschedule onto a booked slot", post, "/api/appointments/"+s.apptSoon+"/reschedule", jsonObject{"roomId": seedRoom1,
		"startTime": clinicUTC(r.t, "2025-03-29", "10:15"), "endTime": clinicUTC(r.t, "2025-03-29", "10:45")})
	e("reschedule a cancelled appointment", post, "/api/appointments/"+s.apptCancelled+"/reschedule", jsonObject{})
	e("reschedule an appointment twice", post, "/api/appointments/"+s.apptDST2+"/reschedule", jsonObject{})
	e("revive a cancelled appointment", put, "/api/appointments/"+s.apptCancelled, jsonObject{"status": "Scheduled"})

	var inv struct {
		InvoiceNumber int `json:"invoiceNumber"`
	}
	r.send(contractCall{path: "/api/invoices/" + s.invoice1}).data(r.t, &inv)
	e("invoice with a taken number", post, "/api/client-invoices", jsonObject{"patientId": s.ben, "invoiceNumber": inv.InvoiceNumber,
		"items": []jsonObject{{"itemType": "other", "quantity": 1, "amount": 1, "notes": "Dup"}}})
	e("invoice more stock than there is", post, "/api/client-invoices", jsonObject{"patientId": s.ben,
		"items": []jsonObject{{"itemType": "product", "itemId": s.cream, "quantity": 500, "amount": 1}}})
	e("invoice with an inactive offer", post, "/api/client-invoices", jsonObject{"patientId": s.ben, "discountId": s.fixedOffer,
		"items": []jsonObject{{"itemType": "other", "quantity": 1, "amount": 10}}})
	e("invoice an unknown patient", post, "/api/client-invoices", jsonObject{"patientId": unknownID,
		"items": []jsonObject{{"itemType": "other", "quantity": 1, "amount": 10}}})
	e("invoice a negative amount", post, "/api/client-invoices", jsonObject{"patientId": s.ben,
		"items": []jsonObject{{"itemType": "other", "quantity": 1, "amount": -10}}})
	e("invoice without items", post, "/api/client-invoices", jsonObject{"patientId": s.ben, "items": []jsonObject{}})
	e("invoice a gift with both a patient and a code", post, "/api/client-invoices", jsonObject{"patientId": s.ben,
		"items": []jsonObject{{"itemType": "gift", "quantity": 1, "amount": 10, "giftCode": "BOTH-1", "giftPatientId": s.cleo}}})
	e("delete an invoice whose gift card was redeemed", del, "/api/client-invoices/"+s.invoice1, nil)
	e("delete a supplier invoice whose stock was sold", del, "/api/supplier-invoices/"+s.supplierInvoice, nil)
	e("supplier invoice without a supplier", post, "/api/supplier-invoices", jsonObject{"items": []jsonObject{{"itemType": "other", "quantity": 1, "amount": 1}}})
	e("supplier invoice for an unknown supplier", post, "/api/supplier-invoices", jsonObject{"supplierId": unknownID,
		"items": []jsonObject{{"itemType": "other", "quantity": 1, "amount": 1}}})
	e("supplier invoice on a patient balance", post, "/api/supplier-invoices", jsonObject{
		"supplierBalanceId": store.DeterministicID("balances", "patient", s.ada, "USD"),
		"items":             []jsonObject{{"itemType": "other", "quantity": 1, "amount": 1}}})
	e("set a negative supplier invoice line", put, "/api/supplier-invoices/"+s.supplierInvoice+"/items/"+s.serumItem, jsonObject{"amount": -5})
	e("redeem a used gift card", post, "/api/gift-cards/redeem", jsonObject{"code": s.giftCode, "patientId": s.cleo})
	e("redeem an unknown gift card", post, "/api/gift-cards/redeem", jsonObject{"code": "NO-SUCH-CARD", "patientId": s.cleo})
	e("redeem a gift card for nobody", post, "/api/gift-cards/redeem", jsonObject{"code": s.giftCode})

	e("pay nothing", post, "/api/client-payments", jsonObject{"patientId": s.ada, "amount": 0, "transactionMethod": "cash"})
	e("pay with an unknown method", post, "/api/client-payments", jsonObject{"patientId": s.ada, "amount": 5, "transactionMethod": "barter"})
	e("pay for an unknown patient", post, "/api/client-payments", jsonObject{"patientId": unknownID, "amount": 5, "transactionMethod": "cash"})
	e("refund an unknown patient", post, "/api/client-refunds", jsonObject{"patientId": unknownID, "amount": 5, "transactionMethod": "cash"})
	e("adjust an unknown patient", post, "/api/client-adjustments", jsonObject{"patientId": unknownID, "amount": 5,
		"transactionMethod": "other", "direction": "incoming", "description": "X"})
	e("adjust without a direction", post, "/api/client-adjustments", jsonObject{"patientId": s.ada, "amount": 5,
		"transactionMethod": "other", "description": "X"})
	e("write off without a description", post, "/api/client-write-offs", jsonObject{"patientId": s.ada, "amount": 5, "direction": "incoming"})
	e("delete a supplier payment as a client payment", del, "/api/client-payments/"+s.acmePayment, nil)
	e("pay an unknown supplier", post, "/api/supplier-payments", jsonObject{"supplierId": unknownID, "amount": 5, "transactionMethod": "cash"})
	e("adjust an unknown supplier", post, "/api/supplier-adjustments", jsonObject{"supplierId": unknownID, "amount": 5,
		"transactionMethod": "other", "direction": "incoming", "description": "X"})
	e("pay an unknown expense", post, "/api/expense-payments", jsonObject{"expenseId": unknownID, "amount": 5, "transactionMethod": "cash"})
	e("write off an unknown expense", post, "/api/expense-write-offs", jsonObject{"expenseId": unknownID, "amount": 5,
		"direction": "incoming", "description": "X"})
	e("delete a client payment as an expense payment", del, "/api/expense-payments/"+s.adaPayment, nil)
	e("pay an unknown employee", post, "/api/employee-payments", jsonObject{"employeeId": unknownID, "amount": 5, "transactionMethod": "cash"})
	e("adjust an unknown employee", post, "/api/employee-adjustments", jsonObject{"employeeId": unknownID, "amount": 5,
		"transactionMethod": "other", "direction": "incoming", "description": "X"})
	e("delete an employee payment as a supplier payment", del, "/api/supplier-payments/"+s.noraPayment, nil)

	e("set a negative salary", post, "/api/employees/"+s.otto+"/salaries", jsonObject{"amount": -1, "effectiveDate": "2025-01-01"})
	e("set a salary for an unknown employee", post, "/api/employees/"+unknownID+"/salaries", jsonObject{"amount": 100, "effectiveDate": "2025-01-01"})
	e("prepare salaries for a backwards period", post, "/api/employee-salaries/prepare", jsonObject{"periodStart": "2025-05-31", "periodEnd": "2025-05-01"})
	e("prepare salaries for a prepared period", post, "/api/employee-salaries/prepare", jsonObject{"periodStart": "2025-03-01", "periodEnd": "2025-03-31"})
	r.run(contractCall{name: "adjust a salary below zero", method: http.MethodPatch, path: "/api/employee-salary-preparations/" + s.noraPrep,
		body: jsonObject{"adjustment": -5000}})
	e("save overlapping shifts", put, "/api/employee-schedules/day", jsonObject{"employeeId": s.nora, "dayOfWeek": 2, "startDate": "2025-03-04",
		"shifts": []jsonObject{{"startTime": "09:00", "endTime": "13:00"}, {"startTime": "12:00", "endTime": "15:00"}}})
	e("save a shift ending before it starts", put, "/api/employee-schedules/day", jsonObject{"employeeId": s.nora, "dayOfWeek": 2,
		"startDate": "2025-03-04", "shifts": []jsonObject{{"startTime": "18:00", "endTime": "09:00"}}})
	e("save a shift with a bad time", put, "/api/employee-schedules/day", jsonObject{"employeeId": s.nora, "dayOfWeek": 2,
		"startDate": "2025-03-04", "shifts": []jsonObject{{"startTime": "9am", "endTime": "5pm"}}})
	e("save a schedule for an unknown employee", put, "/api/employee-schedules/day", jsonObject{"employeeId": unknownID, "dayOfWeek": 2,
		"startDate": "2025-03-04", "shifts": []jsonObject{{"startTime": "09:00", "endTime": "13:00"}}})
	e("request time off ending before it starts", post, "/api/employee-schedule-changes", jsonObject{"employeeId": s.sam, "type": "timeoff",
		"startDate": "2025-05-02", "endDate": "2025-05-01"})
	e("request overtime without times", post, "/api/employee-schedule-changes", jsonObject{"employeeId": s.sam, "type": "overtime",
		"startDate": "2025-05-02", "endDate": "2025-05-02"})
	e("set an unknown schedule change status", post, "/api/employee-schedule-changes/"+s.timeoff+"/status", jsonObject{"status": "maybe"})
	e("update time off to end before it starts", put, "/api/employee-schedule-changes/"+s.timeoff, jsonObject{"endDate": "2025-03-01"})
	e("create a holiday ending before it starts", post, "/api/holidays", jsonObject{"name": "Test Backwards", "startDate": "2025-05-02", "endDate": "2025-05-01"})
	e("create a holiday without a name", post, "/api/holidays", jsonObject{"startDate": "2025-05-02", "endDate": "2025-05-02"})
	e("move a holiday to end before it starts", put, "/api/holidays/"+s.holiday, jsonObject{"endDate": "2025-01-01"})

	e("mark another user's notification read", put, "/api/notifications/"+s.nurseNotif+"/read", nil)
	e("delete another user's notification", del, "/api/notifications/"+s.nurseNotif, nil)
}

// writeErrorSweep sends every write route a malformed body, an empty body or
// ids that match nothing.
func (r *contractRun) writeErrorSweep() {
	malformed := `{"broken"`
	for _, line := range r.routes {
		method, pattern, _ := strings.Cut(line, " ")
		if method == http.MethodGet || !strings.HasPrefix(pattern, "/api/") || strings.HasPrefix(pattern, "/api/auth/") ||
			r.resource[line] == "sync" || (buildmode.Cloud && pattern == "/api/cloud-restore") ||
			pattern == "/api/versions" || pattern == "/api/update/peer" {
			continue
		}
		path := fillUnknownParams(pattern)
		hasBody := method != http.MethodDelete
		build := r.build[line]
		if hasBody {
			r.run(contractCall{name: line + " with a malformed body", method: method, path: path, raw: &malformed, build: build})
		}
		switch {
		case strings.Contains(pattern, "/:") && hasBody:
			r.run(contractCall{name: line + " with unknown ids", method: method, path: path, body: jsonObject{}, build: build})
		case strings.Contains(pattern, "/:"):
			r.run(contractCall{name: line + " with unknown ids", method: method, path: path, build: build})
		default:
			r.run(contractCall{name: line + " with an empty body", method: method, path: path, body: jsonObject{}, build: build})
		}
	}
}

// fillUnknownParams gives every path parameter a value that matches no record.
func fillUnknownParams(pattern string) string {
	segs := strings.Split(pattern, "/")
	for i, s := range segs {
		switch {
		case s == ":name":
			segs[i] = "missing-role"
		case strings.HasPrefix(s, ":"):
			segs[i] = unknownID
		}
	}
	return strings.Join(segs, "/")
}

func (r *contractRun) authCases(s *contractScenario) {
	login := func(name, username, password string, opts ...callOpt) *contractReply {
		c := contractCall{name: name, method: http.MethodPost, path: "/api/auth/login", asCaller: anonymousActor,
			body: jsonObject{"username": username, "password": password}}
		for _, o := range opts {
			o(&c)
		}
		return r.run(c)
	}
	login("the super-admin signs in with another password", "super-admin", "another-pw")
	login("sign in with a wrong password", "admin.example", "wrong-pw")
	login("sign in as an unknown user", "nobody.example", "whatever")
	login("sign in without a password", "admin.example", "  ")
	login("sign in to a deactivated account", "retired.example", "retired-pw")
	login("the first sign-in sets a missing password", "no.password", "first-pw")
	login("a later sign-in must use that password", "no.password", "second-pw")
	login("sign in with padded credentials", "  admin.example ", " admin-example-pw ")
	malformed := `{"username":`
	r.run(contractCall{name: "sign in with a malformed body", method: http.MethodPost, path: "/api/auth/login", asCaller: anonymousActor, raw: &malformed})

	// Ten attempts a minute per address; the eleventh is refused.
	for i := 1; i <= 11; i++ {
		c := contractCall{name: "rate limit attempt", method: http.MethodPost, path: "/api/auth/login", asCaller: anonymousActor,
			body: jsonObject{"username": "nobody.example", "password": "x"}, remote: "10.250.0.1:40000"}
		if i < 11 {
			r.send(c)
			continue
		}
		r.run(c)
	}

	session := r.login("a session to end", "admin.example", "admin-example-pw")
	r.run(contractCall{name: "verify a live session", path: "/api/auth/verify", asCaller: session})
	r.run(contractCall{name: "sign out", method: http.MethodPost, path: "/api/auth/logout", asCaller: session})
	r.run(contractCall{name: "a signed-out token is refused", path: "/api/auth/verify", asCaller: session})
	r.run(contractCall{name: "sign out without a token", method: http.MethodPost, path: "/api/auth/logout", asCaller: anonymousActor})
	r.run(contractCall{name: "verify without a token", path: "/api/auth/verify", asCaller: anonymousActor})
	r.run(contractCall{name: "verify with a malformed token", path: "/api/auth/verify", asCaller: &contractActor{name: "malformed token", token: "not-a-jwt"}})
	r.run(contractCall{name: "a deactivated user's old token is refused", path: "/api/auth/me", asCaller: s.retired})
}

func (r *contractRun) serverCases(s *contractScenario) {
	origin := "http://clinic.example"
	// The built server serves the dashboard itself and answers no cross-origin
	// caller; every response carries the hardening headers.
	hardening := []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "X-Robots-Tag"}
	r.run(contractCall{file: "headers", name: "a GET with an Origin", path: "/api/rooms/dropdown", header: map[string]string{"Origin": origin},
		capture: append([]string{"Access-Control-Allow-Origin", "Vary", "Content-Type"}, hardening...), orderedCase: true})
	r.run(contractCall{file: "headers", name: "a CORS preflight", method: http.MethodOptions, path: "/api/patients", asCaller: anonymousActor,
		header: map[string]string{"Origin": origin, "Access-Control-Request-Method": "PUT", "Access-Control-Request-Headers": "authorization,content-type"},
		capture: append([]string{"Access-Control-Allow-Origin", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
			"Access-Control-Max-Age", "Vary"}, hardening...)})
	r.run(contractCall{file: "headers", name: "an unauthenticated API call", path: "/api/patients", asCaller: anonymousActor,
		capture: append([]string{"Content-Type", "WWW-Authenticate"}, hardening...)})
	r.run(contractCall{file: "headers", name: "a validation error", method: http.MethodPost, path: "/api/rooms", body: jsonObject{},
		capture: append([]string{"Content-Type"}, hardening...)})

	r.run(contractCall{name: "health", path: "/health", asCaller: anonymousActor, capture: append([]string{"Content-Type"}, hardening...)})
	r.run(contractCall{name: "robots.txt", path: "/robots.txt", asCaller: anonymousActor, capture: []string{"Content-Type", "X-Robots-Tag"}})
	r.run(contractCall{name: "the app root", path: "/", asCaller: anonymousActor, capture: append([]string{"Content-Security-Policy"}, hardening...)})
	r.run(contractCall{name: "a deep link into the app", path: "/patients/" + s.ada + "/details", asCaller: anonymousActor})
	r.run(contractCall{name: "an unknown API path without a token", path: "/api/no-such-route", asCaller: anonymousActor})
	r.run(contractCall{name: "an unknown API path with a token", path: "/api/no-such-route"})
	r.run(contractCall{name: "the bare API prefix", path: "/api"})
	r.run(contractCall{name: "an unknown method on a known route", method: http.MethodPatch, path: "/api/patients", body: jsonObject{}})
	r.run(contractCall{name: "a missing file", path: "/files/missing.pdf"})
	r.run(contractCall{name: "a file without a token", path: "/files/missing.pdf", asCaller: anonymousActor})

	keys := func(rep *contractReply) any {
		out := jsonObject{"keys": rawJSONKeys(r.t, rep.body)}
		var env struct{ Data json.RawMessage }
		if json.Unmarshal(rep.body, &env) == nil && strings.HasPrefix(string(env.Data), "{") {
			out["dataKeys"] = rawJSONKeys(r.t, env.Data)
		}
		return out
	}
	env := func(name string, c contractCall) {
		c.file, c.name, c.inspect = "envelope", name, keys
		r.run(c)
	}
	env("a record", contractCall{path: "/api/patients/" + s.ada})
	env("a list", contractCall{path: "/api/rooms"})
	env("a list without a total", contractCall{path: "/api/rooms/dropdown"})
	env("a write", contractCall{method: http.MethodPut, path: "/api/rooms/" + s.room, body: jsonObject{"type": "Consultation"}})
	env("a delete", contractCall{method: http.MethodDelete, path: "/api/notifications/" + s.notifRead})
	env("a not-found error", contractCall{path: "/api/patients/" + unknownID})
	env("a validation error", contractCall{method: http.MethodPost, path: "/api/rooms", body: jsonObject{}})
	env("a sign-in error", contractCall{path: "/api/patients", asCaller: anonymousActor})
	env("a permission error", contractCall{path: "/api/audit-log", asCaller: s.nurse})
	env("an unknown API path", contractCall{path: "/api/no-such-route"})
	env("a login", contractCall{method: http.MethodPost, path: "/api/auth/login", asCaller: anonymousActor,
		body: jsonObject{"username": "admin.example", "password": "admin-example-pw"}})
}

// pdf records a PDF route's answer and then the file it points to, and
// returns the file's path.
func (r *contractRun) pdf(name, path string) string {
	rep := r.run(contractCall{name: name, path: path})
	if rep == nil || rep.status != http.StatusOK {
		return ""
	}
	var out struct {
		URL string `json:"url"`
	}
	rep.data(r.t, &out)
	r.run(contractCall{name: "download: " + name, path: out.URL, file: r.resource[r.route(http.MethodGet, path)]})
	return out.URL
}

func (r *contractRun) pdfCases(s *contractScenario) {
	w := r.dateWindows()
	file := r.pdf("invoice PDF", "/api/invoices/"+s.invoice1+"/pdf")
	// Any signed-in user can download a file made for someone else.
	r.run(contractCall{name: "the nurse downloads another user's invoice PDF", path: file, asCaller: s.nurse, file: "invoices"})
	r.run(contractCall{name: "download an invoice PDF without a token", path: file, asCaller: anonymousActor, file: "invoices"})
	r.pdf("supplier invoice PDF", "/api/invoices/"+s.supplierInvoice+"/pdf")
	r.run(contractCall{name: "PDF of an unknown invoice", path: "/api/invoices/" + unknownID + "/pdf"})
	r.pdf("day schedule PDF", "/api/appointments/pdf?date=2025-03-31")
	r.pdf("week schedule PDF", "/api/appointments/pdf?date=2025-10-27&range=week")
	r.run(contractCall{name: "schedule PDF without a date", path: "/api/appointments/pdf"})
	r.pdf("revenue report PDF", withQuery("/api/reports/revenue/pdf", "from", w.nowFrom, "to", w.nowTo))
	r.pdf("revenue report PDF by procedure", withQuery("/api/reports/revenue/pdf", "from", w.nowFrom, "to", w.nowTo, "level", "procedure"))
	r.pdf("expenses report PDF", withQuery("/api/reports/expenses/pdf", "from", w.nowFrom, "to", w.nowTo))
	r.pdf("analytics report PDF", withQuery("/api/analytics/report/pdf", "from", w.nowFrom, "to", w.nowTo))
	// Instant bounds name the file by the clinic-local days they cover.
	r.pdf("revenue report PDF between clinic midnights", withQuery("/api/reports/revenue/pdf", "from", w.rfcFrom, "to", w.rfcTo))
	r.pdf("analytics report PDF between clinic midnights", withQuery("/api/analytics/report/pdf", "from", w.rfcFrom, "to", w.rfcTo))
	r.run(contractCall{name: "schedule PDF with a malformed date", path: "/api/appointments/pdf?date=31-03-2025"})
	r.run(contractCall{name: "revenue report PDF with a malformed range", path: "/api/reports/revenue/pdf?from=2025&to=2025-05-31"})
	r.run(contractCall{name: "expenses report PDF with a malformed range", path: "/api/reports/expenses/pdf?from=2025-05-01&to=2025-05"})
	r.run(contractCall{name: "analytics report PDF with a malformed range", path: "/api/analytics/report/pdf?to=yesterday"})
	r.run(contractCall{name: "staff asks for an invoice PDF", path: "/api/invoices/" + s.invoice1 + "/pdf", asCaller: s.staff})
	r.run(contractCall{name: "the nurse asks for an invoice PDF", path: "/api/invoices/" + s.invoice1 + "/pdf", asCaller: s.nurse})
}

// buildCases are the routes of one build only: the cloud's machine-to-machine
// endpoints and the two variants of cloud-restore.
func (r *contractRun) buildCases(s *contractScenario) {
	r.run(contractCall{name: "cloud restore without a peer", method: http.MethodPost, path: "/api/cloud-restore", body: jsonObject{}, build: "clinic"})
	r.run(contractCall{name: "cloud restore as a nurse", method: http.MethodPost, path: "/api/cloud-restore", body: jsonObject{}, asCaller: s.nurse, build: "clinic"})

	machine := func(h ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i+1 < len(h); i += 2 {
			m[h[i]] = h[i+1]
		}
		return m
	}
	secret := machine("X-Sync-Secret", testSyncSecret, "X-Sync-Version", buildmode.Version)
	noAuth := anonymousActor
	cloud := func(name, method, path string, h map[string]string, body any) {
		r.run(contractCall{name: name, method: method, path: path, header: h, body: body, asCaller: noAuth, build: "cloud"})
	}
	cloud("sync status", http.MethodGet, "/api/sync/status", secret, nil)
	cloud("sync status with the query secret", http.MethodGet, "/api/sync/status?sync_secret="+testSyncSecret, nil, nil)
	cloud("sync status with a wrong secret", http.MethodGet, "/api/sync/status", machine("X-Sync-Secret", "wrong-secret"), nil)
	cloud("pull the first changes", http.MethodGet, "/api/sync/pull?since=0&limit=1", secret, nil)
	cloud("pull with an old version", http.MethodGet, "/api/sync/pull?since=0&limit=1",
		machine("X-Sync-Secret", testSyncSecret, "X-Sync-Version", "0.0.0-old"), nil)
	cloud("push nothing", http.MethodPost, "/api/sync/push", secret, jsonObject{"rows": []jsonObject{}})
	malformed := `{"rows":`
	r.run(contractCall{name: "push a malformed body", method: http.MethodPost, path: "/api/sync/push", header: secret, raw: &malformed, asCaller: noAuth, build: "cloud"})
	cloud("confirm a stale sync session", http.MethodPost, "/api/sync/ready", secret, jsonObject{"session_token": "stale-token"})
	cloud("fail a stale sync session", http.MethodPost, "/api/sync/failed", secret, jsonObject{"session_token": "stale-token"})
	cloud("publish an incomplete version", http.MethodPost, "/api/versions", machine("X-Publish-Secret", testPublishSecret),
		jsonObject{"version": "9.9.9", "platform": "plan9"})
	cloud("publish with a wrong secret", http.MethodPost, "/api/versions", machine("X-Publish-Secret", "wrong-secret"), jsonObject{})
	cloud("peer update from another version", http.MethodPost, "/api/update/peer",
		machine("X-Sync-Secret", testSyncSecret, "X-Sync-Version", "0.0.0-old"), nil)
	cloud("peer update from this version", http.MethodPost, "/api/update/peer", secret, nil)
	cloud("cloud restore without a checksum", http.MethodPost, "/api/cloud-restore", secret, jsonObject{})
	cloud("cloud restore with a wrong secret", http.MethodPost, "/api/cloud-restore", machine("X-Sync-Secret", "wrong-secret"), jsonObject{})
	r.run(contractCall{name: "cloud restore with a token", method: http.MethodPost, path: "/api/cloud-restore", body: jsonObject{}, build: "cloud"})
	// Last: a second sync stream takes over the critical-sync session.
	r.run(contractCall{name: "open the sync stream", path: "/api/sync/events", header: secret, asCaller: noAuth, build: "cloud", stream: 2})
}
