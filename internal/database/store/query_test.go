package store

import (
	"strings"
	"testing"
)

func TestBoolToInt(t *testing.T) {
	if BoolToInt(true) != 1 {
		t.Errorf("true -> 1")
	}
	if BoolToInt(false) != 0 {
		t.Errorf("false -> 0")
	}
}

func TestListParams_FilterClause(t *testing.T) {
	t.Run("empty filter returns empty clause and nil args", func(t *testing.T) {
		lp := ListParams{Filter: ""}
		c, a := lp.FilterClause("name", "code")
		if c != "" {
			t.Errorf("clause = %q want empty", c)
		}
		if a != nil {
			t.Errorf("args = %v want nil", a)
		}
	})

	t.Run("single column", func(t *testing.T) {
		lp := ListParams{Filter: "abc"}
		c, a := lp.FilterClause("name")
		if c != `(name LIKE ? ESCAPE '\')` {
			t.Errorf("clause = %q", c)
		}
		if len(a) != 1 || a[0] != "%abc%" {
			t.Errorf("args = %v", a)
		}
	})

	t.Run("multiple columns OR'd, args repeated", func(t *testing.T) {
		lp := ListParams{Filter: "x"}
		c, a := lp.FilterClause("a", "b", "c")
		if c != `(a LIKE ? ESCAPE '\' OR b LIKE ? ESCAPE '\' OR c LIKE ? ESCAPE '\')` {
			t.Errorf("clause = %q", c)
		}
		if len(a) != 3 {
			t.Fatalf("want 3 args got %d", len(a))
		}
		for i, v := range a {
			if v != "%x%" {
				t.Errorf("args[%d]=%v want %%x%%", i, v)
			}
		}
	})

	t.Run("filter escapes LIKE wildcards so they match literally", func(t *testing.T) {
		// %, _ and \ are escaped with a backslash; the clause declares ESCAPE '\'
		// so the search matches the literal characters instead of wildcards.
		lp := ListParams{Filter: `10%_y\z`}
		_, a := lp.FilterClause("name")
		if a[0] != `%10\%\_y\\z%` {
			t.Errorf(`got %v, want %%10\%%\_y\\z%%`, a[0])
		}
	})

	t.Run("empty cols slice with filter returns empty parens", func(t *testing.T) {
		lp := ListParams{Filter: "x"}
		c, a := lp.FilterClause()
		if c != "()" {
			t.Errorf("clause = %q", c)
		}
		if a != nil {
			t.Errorf("args = %v", a)
		}
	})
}

func TestListParams_OrderClause(t *testing.T) {
	allowed := map[string]string{
		"name":      "name",
		"createdAt": "created_at",
	}

	cases := []struct {
		name     string
		lp       ListParams
		fallback string
		want     string
	}{
		{"empty sort uses fallback", ListParams{}, "name ASC", " ORDER BY name ASC"},
		{"empty sort and empty fallback returns empty", ListParams{}, "", ""},
		{"unknown sort uses fallback", ListParams{Sort: "ssn"}, "name ASC", " ORDER BY name ASC"},
		{"known sort default ASC", ListParams{Sort: "name"}, "id", " ORDER BY name ASC"},
		{"known sort with desc", ListParams{Sort: "createdAt", Order: "desc"}, "id", " ORDER BY created_at DESC"},
		{"known sort with DESC uppercase", ListParams{Sort: "createdAt", Order: "DESC"}, "id", " ORDER BY created_at DESC"},
		{"order without sort uses fallback", ListParams{Order: "desc"}, "id", " ORDER BY id"},
		{"unknown order falls back to ASC", ListParams{Sort: "name", Order: "sideways"}, "id", " ORDER BY name ASC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.lp.OrderClause(allowed, tc.fallback)
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestListParams_PaginationClause(t *testing.T) {
	cases := []struct {
		name    string
		lp      ListParams
		want    string
		wantSub []string
	}{
		{"zero limit returns empty", ListParams{Limit: 0, Offset: 0}, "", nil},
		{"negative limit returns empty", ListParams{Limit: -5, Offset: 100}, "", nil},
		{"limit only, offset zero", ListParams{Limit: 10, Offset: 0}, " LIMIT 10", nil},
		{"limit + offset", ListParams{Limit: 25, Offset: 50}, " LIMIT 25 OFFSET 50", nil},
		{"negative offset omitted", ListParams{Limit: 10, Offset: -3}, " LIMIT 10", nil},
		{"large numbers", ListParams{Limit: 1000000, Offset: 999999}, " LIMIT 1000000 OFFSET 999999", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.lp.PaginationClause()
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
			for _, s := range tc.wantSub {
				if !strings.Contains(got, s) {
					t.Errorf("missing %q in %q", s, got)
				}
			}
		})
	}
}
