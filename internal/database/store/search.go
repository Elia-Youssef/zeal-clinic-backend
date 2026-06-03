package store

import (
	"fmt"
	"slices"
	"strings"
)

type SearchTarget struct {
	Table  string
	Scope  string
	Fields []string
}

var searchTargets = []SearchTarget{
	{Table: "patients", Scope: "patients:read", Fields: []string{"first_name", "middle_name", "last_name", "contact", "email"}},
	{Table: "suppliers", Scope: "suppliers:read", Fields: []string{"name", "contact", "email"}},
	{Table: "procedures", Scope: "procedures:read", Fields: []string{"name", "remarks"}},
	{Table: "products", Scope: "products:read", Fields: []string{"name"}},
	{Table: "employees", Scope: "employees:read", Fields: []string{"first_name", "last_name", "contact", "email"}},
}

const searchLimit = 3

type SearchHit map[string]any
type SearchResults map[string][]SearchHit

func SearchAll(query string, scopes []string) (SearchResults, error) {
	results := SearchResults{}
	if query == "" {
		return results, nil
	}
	like := "%" + query + "%"

	for _, t := range searchTargets {
		if !slices.Contains(scopes, t.Scope) {
			continue
		}
		hits, err := searchTable(t, like)
		if err != nil {
			return nil, fmt.Errorf("search %s: %w", t.Table, err)
		}
		results[t.Table] = hits
	}
	return results, nil
}

func searchTable(t SearchTarget, like string) ([]SearchHit, error) {
	conds := make([]string, 0, len(t.Fields))
	args := make([]any, 0, len(t.Fields))
	for _, f := range t.Fields {
		conds = append(conds, f+" LIKE ?")
		args = append(args, like)
	}
	q := fmt.Sprintf("SELECT id, %s FROM %s WHERE %s LIMIT %d",
		strings.Join(t.Fields, ", "),
		t.Table,
		strings.Join(conds, " OR "),
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
