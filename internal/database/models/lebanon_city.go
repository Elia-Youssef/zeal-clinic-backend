package models

import (
	"database/sql"
	"errors"
)

type LebanonCity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Governorate string `json:"governorate"`
	District    string `json:"district"`
}

type LebanonCityList []LebanonCity

func (l *LebanonCityList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil LebanonCity rows")
	}
	*l = LebanonCityList{}
	for rows.Next() {
		var item LebanonCity
		if err := rows.Scan(&item.ID, &item.Name, &item.Governorate, &item.District); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (l *LebanonCity) GetByID() error {
	return RDB.QueryRow("SELECT id, name, governorate, district FROM lebanon_cities WHERE id = ?", l.ID).Scan(&l.ID, &l.Name, &l.Governorate, &l.District)
}

func (l *LebanonCityList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("name", "governorate", "district"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM lebanon_cities"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT id, name, governorate, district FROM lebanon_cities` + where + ` ORDER BY governorate, district, name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	l.ScanRows(rows)
	return total, nil
}
