package models

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Patient struct {
	ID                    string  `json:"id"`
	FirstName             string  `json:"firstName"`
	MiddleName            string  `json:"middleName"`
	LastName              string  `json:"lastName"`
	Gender                string  `json:"gender"`
	DateOfBirth           Date    `json:"dateOfBirth"`
	Contact               string  `json:"contact"`
	Email                 string  `json:"email"`
	EmergencyContactName  string  `json:"emergencyContactName"`
	EmergencyContactPhone string  `json:"emergencyContactPhone"`
	Weight                float64 `json:"weight"`
	Height                float64 `json:"height"`
	BloodType             string  `json:"bloodType"`
	Address               string  `json:"address"`
	Notes                 string  `json:"notes"`
	CreatedAt             Date    `json:"createdAt"`
	UpdatedAt             Date    `json:"updatedAt"`
}

func (p *Patient) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(p.FirstName, "First name"); msg != "" {
		e["firstName"] = msg
	}
	if msg := validation.Required(p.LastName, "Last name"); msg != "" {
		e["lastName"] = msg
	}
	if msg := validation.Required(p.Gender, "Gender"); msg != "" {
		e["gender"] = msg
	} else if msg := validation.OneOf(p.Gender, []string{"Male", "Female"}, "Gender"); msg != "" {
		e["gender"] = msg
	}
	if msg := validation.Required(string(p.DateOfBirth), "Date of birth"); msg != "" {
		e["dateOfBirth"] = msg
	} else if msg := validation.Date(string(p.DateOfBirth)); msg != "" {
		e["dateOfBirth"] = msg
	}
	if msg := validation.Required(p.Contact, "Contact phone"); msg != "" {
		e["contact"] = msg
	} else if msg := validation.Phone(p.Contact); msg != "" {
		e["contact"] = msg
	}
	if msg := validation.Email(p.Email); msg != "" {
		e["email"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const patientColumnsNoId = `first_name, middle_name, last_name, gender, date_of_birth,
	contact, email, emergency_contact_name, emergency_contact_phone,
	weight, height, blood_type, address, notes, created_at, updated_at`
const patientColumns = `id, ` + patientColumnsNoId

type PatientList []Patient

func (p *Patient) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil patient row")
	}
	return row.Scan(
		&p.ID, &p.FirstName, &p.MiddleName, &p.LastName, &p.Gender, &p.DateOfBirth,
		&p.Contact, &p.Email, &p.EmergencyContactName, &p.EmergencyContactPhone,
		&p.Weight, &p.Height, &p.BloodType,
		&p.Address, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
	)
}

func (l *PatientList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil patient rows")
	}
	*l = PatientList{}
	for rows.Next() {
		var item Patient
		err := rows.Scan(
			&item.ID, &item.FirstName, &item.MiddleName, &item.LastName, &item.Gender, &item.DateOfBirth,
			&item.Contact, &item.Email, &item.EmergencyContactName, &item.EmergencyContactPhone,
			&item.Weight, &item.Height, &item.BloodType,
			&item.Address, &item.Notes, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (p *Patient) GetAll(params ListParams) ([]Patient, int, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("first_name", "middle_name", "last_name", "contact", "email"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM patients"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + patientColumns + ` FROM patients` + where + ` ORDER BY first_name, last_name` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var list PatientList
	if err := list.ScanRows(rows); err != nil {
		return nil, 0, err
	}
	return list, total, rows.Err()
}

func GetPatientDropdown(params ListParams) ([]DropdownItem, error) {
	where := ""
	var args []interface{}
	if fc, fa := params.FilterClause("first_name", "last_name", "contact"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}
	query := `SELECT id, first_name || ' ' || last_name as name FROM patients` + where + ` ORDER BY first_name, last_name` + params.PaginationClause()
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

func (p *Patient) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+patientColumns+` FROM patients WHERE id = ?`, id)
	return p.ScanRow(row)
}

func (p *Patient) Create() error {
	p.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	_, err := DB.Exec(`INSERT INTO patients (
		id, first_name, middle_name, last_name, gender, date_of_birth,
		contact, email, emergency_contact_name, emergency_contact_phone,
		weight, height, blood_type, address, notes, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.FirstName, p.MiddleName, p.LastName, p.Gender, p.DateOfBirth,
		p.Contact, p.Email, p.EmergencyContactName, p.EmergencyContactPhone,
		p.Weight, p.Height, p.BloodType,
		p.Address, p.Notes, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return err
	}

	// Create a balance for the patient in each currency
	var currencies CurrencyList
	if _, err := currencies.GetAll(ListParams{}); err == nil {
		entityName := p.FirstName + " " + p.LastName
		for _, cur := range currencies {
			bal := Balance{
				EntityType: "patient",
				EntityID:   &p.ID,
				EntityName: entityName,
				CurrencyID: cur.ID,
			}
			bal.GetOrCreate()
		}
	}

	return nil
}

func (p *Patient) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"firstName": "first_name", "middleName": "middle_name", "lastName": "last_name",
		"gender": "gender", "dateOfBirth": "date_of_birth", "contact": "contact", "email": "email",
		"emergencyContactName": "emergency_contact_name", "emergencyContactPhone": "emergency_contact_phone",
		"weight": "weight", "height": "height", "bloodType": "blood_type",
		"address": "address", "notes": "notes",
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
		return p.GetByID(p.ID)
	}

	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, p.ID)
	if _, err := DB.Exec("UPDATE patients SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}

	return p.GetByID(p.ID)
}

func (p *Patient) Delete() error {
	res, err := DB.Exec("DELETE FROM patients WHERE id = ?", p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (p *Patient) GetByPhone(phone string) error {
	row := RDB.QueryRow(`SELECT `+patientColumns+` FROM patients WHERE contact = ?`, phone)
	return p.ScanRow(row)
}
