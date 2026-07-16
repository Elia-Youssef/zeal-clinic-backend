// Package scopes is the single source of truth for API permission scopes.
// Leaf package (no internal imports) so store and migrations can both use it.
package scopes

import "slices"

var All = []string{
	"allergies:read", "allergies:write", "allergies:delete",
	"analytics:read",
	"appointments:read", "appointments:write", "appointments:delete",
	"audit:read",
	"balances:read",
	"cloud-restore:write",
	"currencies:read", "currencies:write", "currencies:delete",
	"discounts:read", "discounts:write", "discounts:delete",
	"employees:read", "employees:write", "employees:delete",
	"employee-payments:read", "employee-payments:write", "employee-payments:delete",
	"employee-salaries:read", "employee-salaries:write", "employee-salaries:delete",
	"expenses:read", "expenses:write", "expenses:delete",
	"hr:read", "hr:write", "hr:delete",
	"invoices:read", "invoices:write", "invoices:delete",
	"medicines:read", "medicines:write", "medicines:delete",
	"patients:read", "patients:write", "patients:delete",
	"patient-allergies:read", "patient-allergies:write",
	"patient-medicines:read", "patient-medicines:write",
	"payments:read", "payments:write", "payments:delete",
	"prescriptions:read", "prescriptions:write", "prescriptions:delete",
	"procedures:read", "procedures:write", "procedures:delete",
	"procedure-categories:read", "procedure-categories:write",
	"procedure-types:read", "procedure-types:write",
	"procedure-allergy-conflicts:read", "procedure-allergy-conflicts:write",
	"products:read", "products:write", "products:delete",
	"product-categories:read", "product-categories:write", "product-categories:delete",
	"product-allergy-conflicts:read", "product-allergy-conflicts:write",
	"reports:read",
	"roles:read", "roles:write",
	"rooms:read", "rooms:write", "rooms:delete",
	"employee-schedules:write", "employee-schedules:delete",
	"suppliers:read", "suppliers:write", "suppliers:delete",
	"update:read", "update:write",
	"users:read", "users:write",
}

// Scopes admin must keep, so it can never lock itself out.
var AdminRequired = []string{"roles:read", "roles:write", "users:read", "users:write"}

func IsValid(s string) bool { return slices.Contains(All, s) }
