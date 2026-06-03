package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// Procedure Allergy Conflict

const procedureAllergyConflictColumnsNoId = `procedure_id, allergy_id, notes, created_at`
const procedureAllergyConflictColumns = `id, ` + procedureAllergyConflictColumnsNoId

type ProcedureAllergyConflict struct {
	ID          string `json:"id"`
	ProcedureID string `json:"procedureId"`
	AllergyID   string `json:"allergyId"`
	Notes       string `json:"notes"`
	CreatedAt   Date   `json:"createdAt"`
	// Joined fields
	AllergyName string `json:"allergyName,omitempty"`
}

func (c *ProcedureAllergyConflict) IsValid() error {
	if msg := validation.Required(c.AllergyID, "Allergy ID"); msg != "" {
		return validation.Errors{"allergyId": msg}
	}
	return nil
}

type ProcedureAllergyConflictList []ProcedureAllergyConflict

func (m *ProcedureAllergyConflict) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ProcedureAllergyConflict row")
	}
	return row.Scan(&m.ID, &m.ProcedureID, &m.AllergyID, &m.Notes, &m.CreatedAt, &m.AllergyName)
}

func (l *ProcedureAllergyConflictList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ProcedureAllergyConflict rows")
	}
	*l = ProcedureAllergyConflictList{}
	for rows.Next() {
		var item ProcedureAllergyConflict
		err := rows.Scan(&item.ID, &item.ProcedureID, &item.AllergyID, &item.Notes, &item.CreatedAt, &item.AllergyName)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (c *ProcedureAllergyConflictList) GetByProcedure(procedureID string, params ListParams) (int, error) {
	var total int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM procedure_allergy_conflicts WHERE procedure_id = ?`, procedureID).Scan(&total); err != nil {
		return 0, err
	}

	rows, err := RDB.Query(`SELECT pac.id, pac.procedure_id, pac.allergy_id, pac.notes, pac.created_at, a.name
		FROM procedure_allergy_conflicts pac JOIN allergies a ON a.id = pac.allergy_id
		WHERE pac.procedure_id = ? ORDER BY a.name`+params.PaginationClause(), procedureID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	c.ScanRows(rows)
	return total, rows.Err()
}

func (c *ProcedureAllergyConflict) Create() error {
	c.ID = uuid.Must(uuid.NewV7()).String()
	c.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO procedure_allergy_conflicts (id, procedure_id, allergy_id, notes, created_at) VALUES (?,?,?,?,?)`,
		c.ID, c.ProcedureID, c.AllergyID, c.Notes, c.CreatedAt)
	return err
}

func (c *ProcedureAllergyConflict) UpdateNotes() error {
	res, err := DB.Exec("UPDATE procedure_allergy_conflicts SET notes = ? WHERE id = ?", c.Notes, c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (c *ProcedureAllergyConflict) Delete() error {
	res, err := DB.Exec("DELETE FROM procedure_allergy_conflicts WHERE id = ?", c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
