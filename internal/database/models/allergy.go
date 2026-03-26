package models

import (
	"database/sql"

	"github.com/google/uuid"
)

type Allergy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
}

type PatientAllergy struct {
	ID        string `json:"id"`
	PatientID string `json:"patientId"`
	AllergyID string `json:"allergyId"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"createdAt"`
	// Joined fields
	AllergyName string `json:"allergyName,omitempty"`
}

type ProcedureAllergyConflict struct {
	ID          string `json:"id"`
	ProcedureID string `json:"procedureId"`
	AllergyID   string `json:"allergyId"`
	Severity    string `json:"severity"`
	Notes       string `json:"notes"`
	CreatedAt   string `json:"createdAt"`
	// Joined fields
	AllergyName string `json:"allergyName,omitempty"`
}

type ProductAllergyConflict struct {
	ID        string `json:"id"`
	SKU       string `json:"sku"`
	AllergyID string `json:"allergyId"`
	Severity  string `json:"severity"`
	Notes     string `json:"notes"`
	CreatedAt string `json:"createdAt"`
	// Joined fields
	AllergyName string `json:"allergyName,omitempty"`
}

// Allergy CRUD

func (a *Allergy) GetAll() ([]Allergy, error) {
	rows, err := DB.Query(`SELECT id, name, description, created_at FROM allergies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Allergy
	for rows.Next() {
		var item Allergy
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *Allergy) GetByID(id string) error {
	return DB.QueryRow(`SELECT id, name, description, created_at FROM allergies WHERE id = ?`, id).
		Scan(&a.ID, &a.Name, &a.Description, &a.CreatedAt)
}

func (a *Allergy) Create() error {
	a.ID = uuid.New().String()
	_, err := DB.Exec(`INSERT INTO allergies (id, name, description, created_at) VALUES (?,?,?,?)`,
		a.ID, a.Name, a.Description, a.CreatedAt)
	return err
}

func (a *Allergy) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"name": "name", "description": "description",
	}
	setClauses := ""
	var args []interface{}
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
		return a.GetByID(a.ID)
	}
	args = append(args, a.ID)
	if _, err := DB.Exec("UPDATE allergies SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}
	return a.GetByID(a.ID)
}

func (a *Allergy) Delete() error {
	res, err := DB.Exec("DELETE FROM allergies WHERE id = ?", a.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Patient Allergies

func (pa *PatientAllergy) GetByPatient(patientID string) ([]PatientAllergy, error) {
	rows, err := DB.Query(`SELECT pa.id, pa.patient_id, pa.allergy_id, pa.notes, pa.created_at, a.name
		FROM patient_allergies pa JOIN allergies a ON a.id = pa.allergy_id
		WHERE pa.patient_id = ? ORDER BY a.name`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []PatientAllergy
	for rows.Next() {
		var item PatientAllergy
		if err := rows.Scan(&item.ID, &item.PatientID, &item.AllergyID, &item.Notes, &item.CreatedAt, &item.AllergyName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (pa *PatientAllergy) Create() error {
	pa.ID = uuid.New().String()
	_, err := DB.Exec(`INSERT INTO patient_allergies (id, patient_id, allergy_id, notes, created_at) VALUES (?,?,?,?,?)`,
		pa.ID, pa.PatientID, pa.AllergyID, pa.Notes, pa.CreatedAt)
	return err
}

func (pa *PatientAllergy) Delete() error {
	res, err := DB.Exec("DELETE FROM patient_allergies WHERE id = ?", pa.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Procedure Allergy Conflicts

func (c *ProcedureAllergyConflict) GetByProcedure(procedureID string) ([]ProcedureAllergyConflict, error) {
	rows, err := DB.Query(`SELECT pac.id, pac.procedure_id, pac.allergy_id, pac.severity, pac.notes, pac.created_at, a.name
		FROM procedure_allergy_conflicts pac JOIN allergies a ON a.id = pac.allergy_id
		WHERE pac.procedure_id = ? ORDER BY a.name`, procedureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ProcedureAllergyConflict
	for rows.Next() {
		var item ProcedureAllergyConflict
		if err := rows.Scan(&item.ID, &item.ProcedureID, &item.AllergyID, &item.Severity, &item.Notes, &item.CreatedAt, &item.AllergyName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *ProcedureAllergyConflict) Create() error {
	c.ID = uuid.New().String()
	_, err := DB.Exec(`INSERT INTO procedure_allergy_conflicts (id, procedure_id, allergy_id, severity, notes, created_at) VALUES (?,?,?,?,?,?)`,
		c.ID, c.ProcedureID, c.AllergyID, c.Severity, c.Notes, c.CreatedAt)
	return err
}

func (c *ProcedureAllergyConflict) Delete() error {
	res, err := DB.Exec("DELETE FROM procedure_allergy_conflicts WHERE id = ?", c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Product Allergy Conflicts

func (c *ProductAllergyConflict) GetBySKU(sku string) ([]ProductAllergyConflict, error) {
	rows, err := DB.Query(`SELECT pac.id, pac.sku, pac.allergy_id, pac.severity, pac.notes, pac.created_at, a.name
		FROM product_allergy_conflicts pac JOIN allergies a ON a.id = pac.allergy_id
		WHERE pac.sku = ? ORDER BY a.name`, sku)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ProductAllergyConflict
	for rows.Next() {
		var item ProductAllergyConflict
		if err := rows.Scan(&item.ID, &item.SKU, &item.AllergyID, &item.Severity, &item.Notes, &item.CreatedAt, &item.AllergyName); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *ProductAllergyConflict) Create() error {
	c.ID = uuid.New().String()
	_, err := DB.Exec(`INSERT INTO product_allergy_conflicts (id, sku, allergy_id, severity, notes, created_at) VALUES (?,?,?,?,?,?)`,
		c.ID, c.SKU, c.AllergyID, c.Severity, c.Notes, c.CreatedAt)
	return err
}

func (c *ProductAllergyConflict) Delete() error {
	res, err := DB.Exec("DELETE FROM product_allergy_conflicts WHERE id = ?", c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
