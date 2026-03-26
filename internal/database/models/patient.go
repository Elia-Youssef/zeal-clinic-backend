package models

import (
	"database/sql"
	"fmt"
	"time"
)

type Patient struct {
	ID                    string  `json:"id"`
	PatientNumber         int     `json:"patientNumber"`
	FirstName             string  `json:"firstName"`
	MiddleName            string  `json:"middleName"`
	LastName              string  `json:"lastName"`
	Gender                string  `json:"gender"`
	DateOfBirth           string  `json:"dateOfBirth"`
	Contact               string  `json:"contact"`
	Email                 string  `json:"email"`
	EmergencyContactName  string  `json:"emergencyContactName"`
	EmergencyContactPhone string  `json:"emergencyContactPhone"`
	Weight                float64 `json:"weight"`
	Height                float64 `json:"height"`
	BP                    string  `json:"bp"`
	BloodType             string  `json:"bloodType"`
	PhysicalActivity      string  `json:"physicalActivity"`
	IsSmoker              bool    `json:"isSmoker"`
	PacksPerDay           float64 `json:"packsPerDay"`
	OnHerbalSupplements   bool    `json:"onHerbalSupplements"`
	OnMedication          bool    `json:"onMedication"`
	MedicationDetails     string  `json:"medicationDetails"`
	OnBloodThinners       bool    `json:"onBloodThinners"`
	OnHRT                 bool    `json:"onHRT"`
	HRTDetails            string  `json:"hrtDetails"`
	Notes                 string  `json:"notes"`
	CreatedAt             string  `json:"createdAt"`
	UpdatedAt             string  `json:"updatedAt"`
}

const patientColumns = `id, patient_number, first_name, middle_name, last_name, gender, date_of_birth,
	contact, email, emergency_contact_name, emergency_contact_phone,
	weight, height, bp, blood_type, physical_activity,
	is_smoker, packs_per_day, on_herbal_supplements, on_medication, medication_details,
	on_blood_thinners, on_hrt, hrt_details, notes, created_at, updated_at`

func (p *Patient) GetAll() ([]Patient, error) {
	rows, err := DB.Query(`SELECT ` + patientColumns + ` FROM patients ORDER BY patient_number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patients []Patient
	for rows.Next() {
		pt, err := scanPatient(rows)
		if err != nil {
			return nil, err
		}
		patients = append(patients, pt)
	}
	return patients, rows.Err()
}

func (p *Patient) GetByID(id string) error {
	row := DB.QueryRow(`SELECT `+patientColumns+` FROM patients WHERE id = ?`, id)
	scanned, err := scanPatientRow(row)
	if err != nil {
		return err
	}
	*p = scanned
	return nil
}

func (p *Patient) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var nextNum int
	err = tx.QueryRow(`UPDATE counters SET value = value + 1 WHERE name = 'patient' RETURNING value`).Scan(&nextNum)
	if err != nil {
		return fmt.Errorf("counter: %w", err)
	}

	p.ID = fmt.Sprintf("PAT-%03d", nextNum)
	p.PatientNumber = nextNum
	now := time.Now().Format(time.RFC3339)
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	_, err = tx.Exec(`INSERT INTO patients (
		id, patient_number, first_name, middle_name, last_name, gender, date_of_birth,
		contact, email, emergency_contact_name, emergency_contact_phone,
		weight, height, bp, blood_type, physical_activity,
		is_smoker, packs_per_day, on_herbal_supplements, on_medication, medication_details,
		on_blood_thinners, on_hrt, hrt_details, notes, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.PatientNumber, p.FirstName, p.MiddleName, p.LastName, p.Gender, p.DateOfBirth,
		p.Contact, p.Email, p.EmergencyContactName, p.EmergencyContactPhone,
		p.Weight, p.Height, p.BP, p.BloodType, p.PhysicalActivity,
		BoolToInt(p.IsSmoker), p.PacksPerDay, BoolToInt(p.OnHerbalSupplements),
		BoolToInt(p.OnMedication), p.MedicationDetails,
		BoolToInt(p.OnBloodThinners), BoolToInt(p.OnHRT), p.HRTDetails,
		p.Notes, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (p *Patient) Update(updates map[string]interface{}) error {
	cols := map[string]string{
		"firstName": "first_name", "middleName": "middle_name", "lastName": "last_name",
		"gender": "gender", "dateOfBirth": "date_of_birth", "contact": "contact", "email": "email",
		"emergencyContactName": "emergency_contact_name", "emergencyContactPhone": "emergency_contact_phone",
		"weight": "weight", "height": "height", "bp": "bp", "bloodType": "blood_type",
		"physicalActivity": "physical_activity",
		"isSmoker": "is_smoker", "packsPerDay": "packs_per_day",
		"onHerbalSupplements": "on_herbal_supplements", "onMedication": "on_medication",
		"medicationDetails": "medication_details", "onBloodThinners": "on_blood_thinners",
		"onHRT": "on_hrt", "hrtDetails": "hrt_details", "notes": "notes",
	}

	setClauses := ""
	var args []interface{}
	for jsonKey, dbCol := range cols {
		if val, ok := updates[jsonKey]; ok {
			if setClauses != "" {
				setClauses += ", "
			}
			switch dbCol {
			case "is_smoker", "on_herbal_supplements", "on_medication", "on_blood_thinners", "on_hrt":
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
	args = append(args, time.Now().Format(time.RFC3339))
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
	scanned, err := scanPatientRow(row)
	if err != nil {
		return err
	}
	*p = scanned
	return nil
}

// scan helpers

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanPatientFields(s scannable) (Patient, error) {
	var p Patient
	var isSmoker, onHerbal, onMed, onBT, onHRT int
	err := s.Scan(
		&p.ID, &p.PatientNumber, &p.FirstName, &p.MiddleName, &p.LastName, &p.Gender, &p.DateOfBirth,
		&p.Contact, &p.Email, &p.EmergencyContactName, &p.EmergencyContactPhone,
		&p.Weight, &p.Height, &p.BP, &p.BloodType, &p.PhysicalActivity,
		&isSmoker, &p.PacksPerDay, &onHerbal, &onMed, &p.MedicationDetails,
		&onBT, &onHRT, &p.HRTDetails, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return p, err
	}
	p.IsSmoker = isSmoker == 1
	p.OnHerbalSupplements = onHerbal == 1
	p.OnMedication = onMed == 1
	p.OnBloodThinners = onBT == 1
	p.OnHRT = onHRT == 1
	return p, nil
}

func scanPatient(rows *sql.Rows) (Patient, error) {
	return scanPatientFields(rows)
}

func scanPatientRow(row *sql.Row) (Patient, error) {
	return scanPatientFields(row)
}
