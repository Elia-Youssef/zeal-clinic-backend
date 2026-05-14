package sync

// TableInfo describes a replicated table. SyncedTables order is parent-first;
// deletes are applied in reverse.
type TableInfo struct {
	Name         string
	PKColumn     string
	NoDelete     bool
	HasUpdatedAt bool
	CacheKeys    []string
}

func (t TableInfo) PK() string {
	if t.PKColumn == "" {
		return "id"
	}
	return t.PKColumn
}

var SyncedTables = []TableInfo{
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

	{Name: "users", HasUpdatedAt: true, CacheKeys: []string{"users"}},
	{Name: "procedures", HasUpdatedAt: true, CacheKeys: []string{"procedures", "discounts"}},
	{Name: "products", CacheKeys: []string{"products", "discounts", "analytics"}},
	{Name: "discounts", HasUpdatedAt: true, CacheKeys: []string{"discounts"}},

	{Name: "employees", HasUpdatedAt: true, CacheKeys: []string{"employees"}},
	{Name: "patients", HasUpdatedAt: true, CacheKeys: []string{"patients", "appointments", "analytics"}},
	{Name: "balances", HasUpdatedAt: true, CacheKeys: []string{"balances"}},

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

	{Name: "appointment_procedures", HasUpdatedAt: true, CacheKeys: []string{"appointments", "analytics"}},
	{Name: "prescription_medicines"},
	{Name: "balance_transactions", NoDelete: true, CacheKeys: []string{"balances", "client-payments", "employee-payments", "supplier-payments", "expense-payments", "analytics"}},
	{Name: "invoices", NoDelete: true, HasUpdatedAt: true, CacheKeys: []string{"invoices", "client-invoices", "supplier-invoices", "analytics"}},

	{Name: "invoice_items", NoDelete: true, CacheKeys: []string{"invoices", "client-invoices", "supplier-invoices", "balances", "analytics"}},
}

var tableSet = func() map[string]TableInfo {
	m := make(map[string]TableInfo, len(SyncedTables))
	for _, t := range SyncedTables {
		m[t.Name] = t
	}
	return m
}()

func IsSyncedTable(name string) (TableInfo, bool) {
	t, ok := tableSet[name]
	return t, ok
}
