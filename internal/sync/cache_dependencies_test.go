package sync

import "testing"

func TestSyncedTableCacheDependencies(t *testing.T) {
	want := map[string][]string{
		"procedure_types":      {"procedure-types", "procedures", "reports"},
		"procedure_categories": {"procedure-categories", "procedures", "appointments", "invoices", "analytics", "reports"},
		"rooms":                {"rooms", "appointments", "analytics"},
		"product_categories":   {"product-categories", "products", "reports"},
		"lebanon_cities":       {"lebanon-cities", "analytics"},
		"procedures":           {"procedures", "discounts", "appointments", "invoices", "analytics", "reports"},
		"products":             {"products", "discounts", "invoices", "analytics", "reports"},
		"discounts":            {"discounts", "invoices", "analytics"},
		"employees":            {"employees", "employee-schedules", "appointments", "analytics"},
		"balances":             {"balances", "invoices", "analytics", "reports"},
		"holidays":             {"holidays", "employee-schedules", "appointments", "analytics"},
		"employee_salaries":    {"employees"},
		"product_prices":       {"products", "analytics"},
		"balance_transactions": {"balances", "client-payments", "employee-payments", "supplier-payments", "expense-payments", "analytics", "reports"},
		"invoices":             {"invoices", "client-invoices", "supplier-invoices", "analytics", "reports"},
		"invoice_items":        {"invoices", "client-invoices", "supplier-invoices", "balances", "analytics", "reports"},
	}

	for table, wantKeys := range want {
		t.Run(table, func(t *testing.T) {
			info, ok := IsSyncedTable(table)
			if !ok {
				t.Fatalf("synced table %q not found", table)
			}
			assertSameCacheKeys(t, info.CacheKeys, wantKeys)
		})
	}
}

func assertSameCacheKeys(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("cache keys = %v, want %v", got, want)
	}
	counts := make(map[string]int, len(want))
	for _, key := range want {
		counts[key]++
	}
	for _, key := range got {
		counts[key]--
	}
	for _, count := range counts {
		if count != 0 {
			t.Fatalf("cache keys = %v, want %v", got, want)
		}
	}
}
