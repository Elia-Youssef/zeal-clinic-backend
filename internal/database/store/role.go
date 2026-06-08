package store

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"clinic-api/internal/scopes"
)

type Role struct {
	Name   string   `json:"name"`
	Label  string   `json:"label"`
	Scopes []string `json:"scopes"`
}

const roleColumns = `name, label, scopes`

type RoleList []Role

func (m *Role) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Role row")
	}
	var scopesStr string
	err := row.Scan(&m.Name, &m.Label, &scopesStr)
	if err != nil {
		return err
	}
	if scopesStr != "" {
		m.Scopes = strings.Split(scopesStr, ",")
	} else {
		m.Scopes = []string{}
	}
	return nil
}

func (l *RoleList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Role rows")
	}
	*l = RoleList{}
	for rows.Next() {
		var item Role
		var scopesStr string
		err := rows.Scan(&item.Name, &item.Label, &scopesStr)
		if err != nil {
			continue
		}
		if scopesStr != "" {
			item.Scopes = strings.Split(scopesStr, ",")
		} else {
			item.Scopes = []string{}
		}
		*l = append(*l, item)
	}
	return nil
}

func (r *RoleList) GetAll(params ListParams) (int, error) {
	where := " WHERE name != 'super-admin'"
	var args []any
	if fc, fa := params.FilterClause("name", "label"); fc != "" {
		where += " AND " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM roles"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"name":  "name",
		"label": "label",
	}, "name")
	query := `SELECT ` + roleColumns + ` FROM roles` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	r.ScanRows(rows)
	return total, nil
}

func GetRoleDropdown(params ListParams) ([]DropdownItem, error) {
	where := " WHERE name != 'super-admin'"
	var args []any
	if fc, fa := params.FilterClause("name", "label"); fc != "" {
		where += " AND " + fc
		args = fa
	}
	query := `SELECT name, label FROM roles` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DropdownItem
	for rows.Next() {
		var item DropdownItem
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Role) GetByName(name string) error {
	return r.ScanRow(RDB.QueryRow(`SELECT `+roleColumns+` FROM roles WHERE name = ?`, name))
}

func (r *Role) Update(updates map[string]any) error {
	if raw, ok := updates["scopes"]; ok {
		if slice, ok := raw.([]any); ok {
			var parts []string
			for _, v := range slice {
				if str, ok := v.(string); ok {
					parts = append(parts, str)
				}
			}
			for _, s := range parts {
				if !scopes.IsValid(s) {
					return fmt.Errorf("%w: Unknown permission %q", ErrValidation, s)
				}
			}
			if r.Name == "admin" {
				for _, req := range scopes.AdminRequired {
					if !slices.Contains(parts, req) {
						return fmt.Errorf("%w: The admin role must keep %s", ErrValidation, req)
					}
				}
			}
			if _, err := DB.Exec("UPDATE roles SET scopes = ? WHERE name = ?", strings.Join(parts, ","), r.Name); err != nil {
				return err
			}
		}
	}
	if label, ok := updates["label"]; ok {
		if _, err := DB.Exec("UPDATE roles SET label = ? WHERE name = ?", label, r.Name); err != nil {
			return err
		}
	}
	return r.GetByName(r.Name)
}
