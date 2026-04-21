package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type ProcedureType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   Date   `json:"createdAt"`
}

func (t *ProcedureType) IsValid() error {
	if msg := validation.Required(t.Name, "Name"); msg != "" {
		return validation.Errors{"name": msg}
	}
	return nil
}

const procedureTypeColumnsNoId = `name, description, created_at`
const procedureTypeColumns = `id, ` + procedureTypeColumnsNoId

type ProcedureTypeList []ProcedureType

func (m *ProcedureType) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ProcedureType row")
	}
	return row.Scan(&m.ID, &m.Name, &m.Description, &m.CreatedAt)
}

func (l *ProcedureTypeList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ProcedureType rows")
	}
	*l = ProcedureTypeList{}
	for rows.Next() {
		var item ProcedureType
		err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (t *ProcedureTypeList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name", "description"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM procedure_types"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT ` + procedureTypeColumns + ` FROM procedure_types` + where + ` ORDER BY name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	t.ScanRows(rows)
	return total, nil
}

func GetProcedureTypeDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, name FROM procedure_types` + where + ` ORDER BY name` + params.PaginationClause()
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

func (t *ProcedureType) GetByID(id string) error {
	return t.ScanRow(RDB.QueryRow(`SELECT `+procedureTypeColumns+` FROM procedure_types WHERE id = ?`, id))
}

func (t *ProcedureType) Create() error {
	t.ID = uuid.Must(uuid.NewV7()).String()
	t.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO procedure_types (`+procedureTypeColumns+`) VALUES (?,?,?,?)`,
		t.ID, t.Name, t.Description, t.CreatedAt)
	return err
}

func (t *ProcedureType) Update(updates map[string]any) error {
	cols := map[string]string{"name": "name", "description": "description"}
	setClauses := ""
	var args []any
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if setClauses != "" {
				setClauses += ", "
			}
			setClauses += dbCol + " = ?"
			args = append(args, val)
		}
	}
	if setClauses == "" {
		return t.GetByID(t.ID)
	}
	args = append(args, t.ID)
	_, err := DB.Exec("UPDATE procedure_types SET "+setClauses+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	return t.GetByID(t.ID)
}

func (t *ProcedureType) Delete() error {
	res, err := DB.Exec("DELETE FROM procedure_types WHERE id = ?", t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
