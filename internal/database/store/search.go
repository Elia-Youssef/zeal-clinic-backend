package store

import (
	"fmt"
	"slices"
	"strings"
)

type SearchTarget struct {
	Table  string
	Scope  string
	Fields []string // columns returned in each hit
	Match  []string // expressions searched; defaults to Fields when empty
}

var searchTargets = []SearchTarget{
	{Table: "patients", Scope: "patients:read", Fields: []string{"first_name", "middle_name", "last_name", "contact", "email"}, Match: []string{patientNameExpr, "contact", "email"}},
	{Table: "suppliers", Scope: "suppliers:read", Fields: []string{"name", "contact", "email"}},
	{Table: "procedures", Scope: "procedures:read", Fields: []string{"name", "remarks"}},
	{Table: "products", Scope: "products:read", Fields: []string{"name"}},
	{Table: "employees", Scope: "employees:read", Fields: []string{"first_name", "last_name", "contact", "email"}, Match: []string{employeeNameExpr, "contact", "email"}},
}

const searchLimit = 5

type SearchHit map[string]any
type SearchResults map[string][]SearchHit

func SearchAll(query string, scopes []string) (SearchResults, error) {
	results := SearchResults{}
	tokens := strings.Fields(query)
	if len(tokens) == 0 {
		return results, nil
	}

	for _, t := range searchTargets {
		if !slices.Contains(scopes, t.Scope) {
			continue
		}
		hits, err := searchTable(t, tokens)
		if err != nil {
			return nil, fmt.Errorf("search %s: %w", t.Table, err)
		}
		results[t.Table] = hits
	}
	return results, nil
}

// searchTable matches every token (AND) against any of the target's fields
// (OR), so a multi-word query like "jaden smith" finds a combined name whose
// words aren't adjacent.
func searchTable(t SearchTarget, tokens []string) ([]SearchHit, error) {
	match := t.Match
	if len(match) == 0 {
		match = t.Fields
	}
	groups := make([]string, 0, len(tokens))
	args := make([]any, 0, len(tokens)*len(match))
	for _, tok := range tokens {
		like := "%" + tok + "%"
		conds := make([]string, 0, len(match))
		for _, f := range match {
			conds = append(conds, f+" LIKE ?")
			args = append(args, like)
		}
		groups = append(groups, "("+strings.Join(conds, " OR ")+")")
	}
	q := fmt.Sprintf("SELECT id, %s FROM %s WHERE %s LIMIT %d",
		strings.Join(t.Fields, ", "),
		t.Table,
		strings.Join(groups, " AND "),
		searchLimit,
	)
	rows, err := RDB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	hits := []SearchHit{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		hit := SearchHit{}
		for i, c := range cols {
			hit[c] = vals[i]
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}
