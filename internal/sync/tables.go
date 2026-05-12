package sync

// SyncedTables enumerates every business table that participates in
// cross-node replication. Order matters for applying inserts/updates
// (parents before children); the slice is grouped by FK dependency level.
// Deletes are applied in reverse.
//
// Excluded by design:
//   - tokens, audit_log: local-only session/audit state, must not leak
//   - sync_log, sync_state, sync_conflicts: sync infrastructure itself
//   - goose_db_version: migration tracker
//   - employee_salary_preparations: local payroll workspace; recompute on cloud if ever needed
//
// NoDelete tables (balance_transactions, invoices, invoice_items) preserve
// financial history: a sync DELETE is rejected on both sides and logged as
// a conflict. Updates are still applied: invoice amounts and void state
// legitimately mutate.
//
// HasUpdatedAt drives the LWW gate. Tables without an updated_at column
// fall through to "trust remote", which is acceptable for low-conflict reference
// data and for append-style tables (balance_transactions, invoice_items)
// where the NoDelete guard already covers the dangerous case.
//
// PKColumn defaults to "id" when empty; roles is the lone exception (PK
// is "name"). Triggers and the apply path consult t.PK() everywhere.
//
// CacheKeys lists the HTTP cache buckets (see internal/api/middleware/cache.go)
// that depend on this table. Apply busts them after a sync write lands so
// cached GETs don't serve stale data; the middleware itself only knows
// about HTTP writes and sync bypasses HTTP entirely. Empty = nothing cached.
type TableInfo struct {
	Name         string
	PKColumn     string
	NoDelete     bool
	HasUpdatedAt bool
	CacheKeys    []string
}

// PK returns the primary-key column name for this table, defaulting to
// "id" when PKColumn isn't set.
func (t TableInfo) PK() string {
	if t.PKColumn == "" {
		return "id"
	}
	return t.PKColumn
}

var SyncedTables = []TableInfo{
	// Level 0: leaves (no FKs to other business tables)
	{Name: "roles", PKColumn: "name", CacheKeys: []string{"roles"}},
	{Name: "currencies", HasUpdatedAt: true, CacheKeys: []string{"currencies"}},
	{Name: "allergies", CacheKeys: []string{"allergies", "patients"}},
	{Name: "medicines", CacheKeys: []string{"medicines"}},
	{Name: "procedure_types", CacheKeys: []string{"procedure-types"}},
	{Name: "procedure_categories", CacheKeys: []string{"procedure-categories"}},
	{Name: "rooms", CacheKeys: []string{"rooms", "appointments"}},
	{Name: "product_categories", CacheKeys: []string{"product-categories"}},
	{Name: "countries", CacheKeys: []string{"countries"}},
	{Name: "lebanon_cities", CacheKeys: []string{"lebanon-cities"}},
	{Name: "expenses", HasUpdatedAt: true, CacheKeys: []string{"expenses"}},
	{Name: "suppliers", HasUpdatedAt: true, CacheKeys: []string{"suppliers"}},

	// Level 1
	{Name: "users", HasUpdatedAt: true, CacheKeys: []string{"users"}},
	{Name: "procedures", HasUpdatedAt: true, CacheKeys: []string{"procedures", "discounts"}},
	{Name: "products", CacheKeys: []string{"products", "discounts", "analytics"}},
	{Name: "discounts", HasUpdatedAt: true, CacheKeys: []string{"discounts"}},

	// Level 2
	{Name: "employees", HasUpdatedAt: true, CacheKeys: []string{"employees"}},
	{Name: "patients", HasUpdatedAt: true, CacheKeys: []string{"patients", "appointments", "analytics"}},
	{Name: "balances", HasUpdatedAt: true, CacheKeys: []string{"balances"}},

	// Level 3
	{Name: "appointments", HasUpdatedAt: true, CacheKeys: []string{"appointments", "analytics"}},
	{Name: "prescriptions", HasUpdatedAt: true},
	{Name: "schedule_availability", HasUpdatedAt: true, CacheKeys: []string{"employee-schedules", "analytics"}},
	{Name: "holidays", HasUpdatedAt: true, CacheKeys: []string{"holidays", "employee-schedules", "analytics"}},
	{Name: "employee_vacations", HasUpdatedAt: true, CacheKeys: []string{"employee-schedules", "analytics"}},
	{Name: "employee_salaries", HasUpdatedAt: true, CacheKeys: []string{"employee-payments", "balances", "analytics"}},
	{Name: "patient_allergies", CacheKeys: []string{"patients"}},
	{Name: "patient_medicines", CacheKeys: []string{"patients"}},
	{Name: "procedure_allergy_conflicts", CacheKeys: []string{"procedures"}},
	{Name: "procedure_prices", CacheKeys: []string{"procedures"}},
	{Name: "product_prices", CacheKeys: []string{"products"}},
	{Name: "product_allergy_conflicts", CacheKeys: []string{"products"}},
	{Name: "notifications"},

	// Level 4
	{Name: "appointment_procedures", HasUpdatedAt: true, CacheKeys: []string{"appointments", "analytics"}},
	{Name: "prescription_medicines"},
	{Name: "balance_transactions", NoDelete: true, CacheKeys: []string{"balances", "client-payments", "employee-payments", "supplier-payments", "expense-payments", "analytics"}},
	{Name: "invoices", NoDelete: true, HasUpdatedAt: true, CacheKeys: []string{"invoices", "client-invoices", "supplier-invoices", "analytics"}},

	// Level 5
	{Name: "invoice_items", NoDelete: true, CacheKeys: []string{"invoices", "client-invoices", "supplier-invoices", "balances", "analytics"}},
}

// tableSet is a quick membership lookup, populated at init.
var tableSet = func() map[string]TableInfo {
	m := make(map[string]TableInfo, len(SyncedTables))
	for _, t := range SyncedTables {
		m[t.Name] = t
	}
	return m
}()

// IsSyncedTable returns the table descriptor and whether the table is synced.
func IsSyncedTable(name string) (TableInfo, bool) {
	t, ok := tableSet[name]
	return t, ok
}
