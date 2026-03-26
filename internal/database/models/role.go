package models

type Role struct {
	Name   string      `json:"name"`
	Label  string      `json:"label"`
	Scopes StringSlice `json:"scopes"`
}

func (r *Role) GetAll() ([]Role, error) {
	rows, err := DB.Query("SELECT name, label, scopes FROM roles ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.Name, &role.Label, &role.Scopes); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *Role) GetByName(name string) error {
	return DB.QueryRow("SELECT name, label, scopes FROM roles WHERE name = ?", name).
		Scan(&r.Name, &r.Label, &r.Scopes)
}

func (r *Role) Update(updates map[string]interface{}) error {
	if scopes, ok := updates["scopes"]; ok {
		if slice, ok := scopes.([]interface{}); ok {
			s := StringSlice{}
			for _, v := range slice {
				if str, ok := v.(string); ok {
					s = append(s, str)
				}
			}
			v, _ := s.Value()
			if _, err := DB.Exec("UPDATE roles SET scopes = ? WHERE name = ?", v, r.Name); err != nil {
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
