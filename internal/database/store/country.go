package store

import (
	"database/sql"
	"errors"
)

type Country struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CountryList []Country

func (l *CountryList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Country rows")
	}
	*l = CountryList{}
	for rows.Next() {
		var item Country
		if err := rows.Scan(&item.ID, &item.Name); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (c *Country) GetByID() error {
	return RDB.QueryRow("SELECT name FROM countries WHERE id = ?", c.ID).Scan(&c.Name)
}

func (l *CountryList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM countries"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT id, name FROM countries` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	l.ScanRows(rows)
	return total, nil
}

func GetCountryDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM countries` + where + ` ORDER BY name` + params.PaginationClause()
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
