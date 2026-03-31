package models

import (
	"database/sql"
	"errors"
	"strings"
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

func (r *Role) GetAll() (RoleList, error) {
	rows, err := DB.Query(`SELECT ` + roleColumns + ` FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list RoleList
	list.ScanRows(rows)
	return list, nil
}

func (r *Role) GetByName(name string) error {
	return r.ScanRow(DB.QueryRow(`SELECT `+roleColumns+` FROM roles WHERE name = ?`, name))
}

func (r *Role) Update(updates map[string]interface{}) error {
	if scopes, ok := updates["scopes"]; ok {
		if slice, ok := scopes.([]interface{}); ok {
			var parts []string
			for _, v := range slice {
				if str, ok := v.(string); ok {
					parts = append(parts, str)
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
