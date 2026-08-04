package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestMutationCacheDependencies(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*echo.Group)
		method string
		path   string
		want   []string
	}{
		{"create discount", SetupDiscountRoutes, http.MethodPost, "/discounts", []string{"discounts", "analytics"}},
		{"update discount", SetupDiscountRoutes, http.MethodPut, "/discounts/id", []string{"discounts", "invoices", "analytics", "reports"}},
		{"delete discount", SetupDiscountRoutes, http.MethodDelete, "/discounts/id", []string{"discounts", "invoices", "analytics", "reports"}},
		{"create employee salary", SetupEmployeeSalaryRoutes, http.MethodPost, "/employees/id/salaries", []string{"employees"}},
		{"update employee salary", SetupEmployeeSalaryRoutes, http.MethodPut, "/employee-salaries/id", []string{"employees"}},
		{"delete employee salary", SetupEmployeeSalaryRoutes, http.MethodDelete, "/employee-salaries/id", []string{"employees"}},
		{"update employee", SetupEmployeeRoutes, http.MethodPut, "/employees/id", []string{"employees", "employee-schedules", "appointments", "analytics"}},
		{"delete employee", SetupEmployeeRoutes, http.MethodDelete, "/employees/id", []string{"employees", "employee-schedules", "appointments", "analytics"}},
		{"create holiday", SetupHRRoutes, http.MethodPost, "/holidays", []string{"holidays", "employee-schedules", "appointments", "analytics"}},
		{"update holiday", SetupHRRoutes, http.MethodPut, "/holidays/id", []string{"holidays", "employee-schedules", "appointments", "analytics"}},
		{"delete holiday", SetupHRRoutes, http.MethodDelete, "/holidays/id", []string{"holidays", "employee-schedules", "appointments", "analytics"}},
		{"create procedure category", SetupProcedureCategoryRoutes, http.MethodPost, "/procedure-categories", []string{"procedure-categories", "reports"}},
		{"update procedure category", SetupProcedureCategoryRoutes, http.MethodPut, "/procedure-categories/id", []string{"procedure-categories", "procedures", "appointments", "invoices", "analytics", "reports"}},
		{"delete procedure category", SetupProcedureCategoryRoutes, http.MethodDelete, "/procedure-categories/id", []string{"procedure-categories", "procedures", "appointments", "invoices", "analytics", "reports"}},
		{"create procedure type", SetupProcedureTypeRoutes, http.MethodPost, "/procedure-types", []string{"procedure-types", "reports"}},
		{"update procedure type", SetupProcedureTypeRoutes, http.MethodPut, "/procedure-types/id", []string{"procedure-types", "procedures", "reports"}},
		{"delete procedure type", SetupProcedureTypeRoutes, http.MethodDelete, "/procedure-types/id", []string{"procedure-types", "procedures", "reports"}},
		{"create procedure", SetupProcedureRoutes, http.MethodPost, "/procedures", []string{"procedures", "reports"}},
		{"update procedure", SetupProcedureRoutes, http.MethodPut, "/procedures/id", []string{"procedures", "discounts", "appointments", "invoices", "analytics", "reports"}},
		{"delete procedure", SetupProcedureRoutes, http.MethodDelete, "/procedures/id", []string{"procedures", "discounts", "appointments", "invoices", "analytics", "reports"}},
		{"create product category", SetupProductCategoryRoutes, http.MethodPost, "/product-categories", []string{"product-categories", "reports"}},
		{"update product category", SetupProductCategoryRoutes, http.MethodPut, "/product-categories/id", []string{"product-categories", "products", "reports"}},
		{"delete product category", SetupProductCategoryRoutes, http.MethodDelete, "/product-categories/id", []string{"product-categories", "products", "reports"}},
		{"create product", SetupProductRoutes, http.MethodPost, "/products", []string{"products", "analytics", "reports"}},
		{"update product", SetupProductRoutes, http.MethodPut, "/products/id", []string{"products", "discounts", "invoices", "analytics", "reports"}},
		{"delete product", SetupProductRoutes, http.MethodDelete, "/products/id", []string{"products", "discounts", "invoices", "analytics", "reports"}},
		{"create room", SetupRoomRoutes, http.MethodPost, "/rooms", []string{"rooms", "appointments", "analytics"}},
		{"update room", SetupRoomRoutes, http.MethodPut, "/rooms/id", []string{"rooms", "appointments", "analytics"}},
		{"delete room", SetupRoomRoutes, http.MethodDelete, "/rooms/id", []string{"rooms", "appointments", "analytics"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureMutationCacheKeys(t, tt.setup, tt.method, tt.path)
			assertSameKeys(t, got, tt.want)
		})
	}
}

func captureMutationCacheKeys(t *testing.T, setup func(*echo.Group), method, path string) []string {
	t.Helper()

	oldScope := scope
	oldCache := cache
	defer func() {
		scope = oldScope
		cache = oldCache
	}()

	passthrough := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	}
	scope = func(string) echo.MiddlewareFunc { return passthrough }

	var captured []string
	cache = func(keys ...string) echo.MiddlewareFunc {
		routeKeys := append([]string(nil), keys...)
		return func(echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				captured = append([]string(nil), routeKeys...)
				return c.NoContent(http.StatusNoContent)
			}
		}
	}

	e := echo.New()
	setup(e.Group(""))
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(method, path, nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("route returned status %d; cache middleware was not reached", recorder.Code)
	}
	if captured == nil {
		t.Fatal("route did not register cache invalidation keys")
	}
	return captured
}

func assertSameKeys(t *testing.T, got, want []string) {
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
