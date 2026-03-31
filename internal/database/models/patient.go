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
	BP                    string  `json:"bp"`
	BloodType             string  `json:"bloodType"`
	OnMedication          bool    `json:"onMedication"`
	MedicationDetails     string  `json:"medicationDetails"`
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
	weight, height, bp, blood_type, on_medication, medication_details, notes, created_at, updated_at`
const patientColumns = `id, ` + patientColumnsNoId

type PatientList []Patient

func (p *Patient) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil patient row")
	}
	var onMed int
	err := row.Scan(
		&p.ID, &p.FirstName, &p.MiddleName, &p.LastName, &p.Gender, &p.DateOfBirth,
		&p.Contact, &p.Email, &p.EmergencyContactName, &p.EmergencyContactPhone,
		&p.Weight, &p.Height, &p.BP, &p.BloodType,
		&onMed, &p.MedicationDetails, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return err
	}
	p.OnMedication = onMed == 1
	return nil
}

func (l *PatientList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil patient rows")
	}
	*l = PatientList{}
	for rows.Next() {
		var item Patient
		var onMed int
		err := rows.Scan(
			&item.ID, &item.FirstName, &item.MiddleName, &item.LastName, &item.Gender, &item.DateOfBirth,
			&item.Contact, &item.Email, &item.EmergencyContactName, &item.EmergencyContactPhone,
			&item.Weight, &item.Height, &item.BP, &item.BloodType,
			&onMed, &item.MedicationDetails, &item.Notes, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			continue
		}
		item.OnMedication = onMed == 1
		*l = append(*l, item)
	}
	return nil
}

func (p *Patient) GetAll() ([]Patient, error) {
	rows, err := DB.Query(`SELECT ` + patientColumns + ` FROM patients ORDER BY first_name, last_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list PatientList
	if err := list.ScanRows(rows); err != nil {
		return nil, err
	}
	return list, rows.Err()
}

func (p *Patient) GetByID(id string) error {
	row := DB.QueryRow(`SELECT `+patientColumns+` FROM patients WHERE id = ?`, id)
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
		weight, height, bp, blood_type, on_medication, medication_details,
		notes, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.FirstName, p.MiddleName, p.LastName, p.Gender, p.DateOfBirth,
		p.Contact, p.Email, p.EmergencyContactName, p.EmergencyContactPhone,
		p.Weight, p.Height, p.BP, p.BloodType,
		BoolToInt(p.OnMedication), p.MedicationDetails,
		p.Notes, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (p *Patient) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"firstName": "first_name", "middleName": "middle_name", "lastName": "last_name",
		"gender": "gender", "dateOfBirth": "date_of_birth", "contact": "contact", "email": "email",
		"emergencyContactName": "emergency_contact_name", "emergencyContactPhone": "emergency_contact_phone",
		"weight": "weight", "height": "height", "bp": "bp", "bloodType": "blood_type",
		"onMedication": "on_medication", "medicationDetails": "medication_details", "notes": "notes",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if setClauses != "" {
				setClauses += ", "
			}
			switch dbCol {
			case "on_medication":
				if b, ok := val.(bool); ok {
					val = BoolToInt(b)
				}
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
	row := DB.QueryRow(`SELECT `+patientColumns+` FROM patients WHERE contact = ?`, phone)
	return p.ScanRow(row)
}
