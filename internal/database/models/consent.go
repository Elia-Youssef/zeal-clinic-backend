package models

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type ConsentTemplate struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Procedure string `json:"procedure"`
	Content   string `json:"content"`
	IsActive  bool   `json:"isActive"`
	CreatedAt string `json:"createdAt"`
}

type ConsentForm struct {
	ID                 string `json:"id"`
	PatientID          string `json:"patientId"`
	PatientProcedureID string `json:"patientProcedureId"`
	TemplateID         string `json:"templateId"`
	VisitID            string `json:"visitId"`
	FormType           string `json:"formType"`
	Title              string `json:"title"`
	Content            string `json:"content"`
	ProcedureName      string `json:"procedureName"`
	SignatureType      string `json:"signatureType"`
	SignatureData      string `json:"signatureData"`
	SignedName         string `json:"signedName"`
	SignedAt           string `json:"signedAt"`
	Status             string `json:"status"`
	CreatedAt          string `json:"createdAt"`
}

// Templates

func (t *ConsentTemplate) GetAll() ([]ConsentTemplate, error) {
	rows, err := DB.Query(`SELECT id, name, type, procedure, content, is_active, created_at
		FROM consent_templates ORDER BY type, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ConsentTemplate
	for rows.Next() {
		var tmpl ConsentTemplate
		var isActive int
		if err := rows.Scan(&tmpl.ID, &tmpl.Name, &tmpl.Type, &tmpl.Procedure, &tmpl.Content, &isActive, &tmpl.CreatedAt); err != nil {
			return nil, err
		}
		tmpl.IsActive = isActive == 1
		list = append(list, tmpl)
	}
	return list, rows.Err()
}

func (t *ConsentTemplate) Create() error {
	t.ID = uuid.New().String()
	t.CreatedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	isActive := 0
	if t.IsActive {
		isActive = 1
	}
	_, err := DB.Exec(`INSERT INTO consent_templates (id, name, type, procedure, content, is_active, created_at)
		VALUES (?,?,?,?,?,?,?)`, t.ID, t.Name, t.Type, t.Procedure, t.Content, isActive, t.CreatedAt)
	return err
}

func (t *ConsentTemplate) Delete() error {
	res, err := DB.Exec("DELETE FROM consent_templates WHERE id = ?", t.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// Consent Forms

func (f *ConsentForm) GetByPatient(patientID string) ([]ConsentForm, error) {
	rows, err := DB.Query(`SELECT id, patient_id, patient_procedure_id, template_id, visit_id, form_type, title, content, procedure_name,
		signature_type, signature_data, signed_name, signed_at, status, created_at
		FROM consent_forms WHERE patient_id = ? ORDER BY created_at DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ConsentForm
	for rows.Next() {
		var form ConsentForm
		if err := rows.Scan(&form.ID, &form.PatientID, &form.PatientProcedureID, &form.TemplateID, &form.VisitID, &form.FormType, &form.Title, &form.Content, &form.ProcedureName,
			&form.SignatureType, &form.SignatureData, &form.SignedName, &form.SignedAt, &form.Status, &form.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, form)
	}
	return list, rows.Err()
}

func (f *ConsentForm) GetByID(id string) error {
	err := DB.QueryRow(`SELECT id, patient_id, patient_procedure_id, template_id, visit_id, form_type, title, content, procedure_name,
		signature_type, signature_data, signed_name, signed_at, status, created_at
		FROM consent_forms WHERE id = ?`, id).Scan(
		&f.ID, &f.PatientID, &f.PatientProcedureID, &f.TemplateID, &f.VisitID, &f.FormType, &f.Title, &f.Content, &f.ProcedureName,
		&f.SignatureType, &f.SignatureData, &f.SignedName, &f.SignedAt, &f.Status, &f.CreatedAt,
	)
	return err
}

func (f *ConsentForm) Create() error {
	f.ID = uuid.New().String()
	f.CreatedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	if f.Status == "" {
		f.Status = "pending"
	}

	_, err := DB.Exec(`INSERT INTO consent_forms (id, patient_id, patient_procedure_id, template_id, visit_id, form_type, title, content, procedure_name,
		signature_type, signature_data, signed_name, signed_at, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.ID, f.PatientID, f.PatientProcedureID, f.TemplateID, f.VisitID, f.FormType, f.Title, f.Content, f.ProcedureName,
		f.SignatureType, f.SignatureData, f.SignedName, f.SignedAt, f.Status, f.CreatedAt,
	)
	return err
}

func (f *ConsentForm) Sign(signatureType, signatureData, signedName string) error {
	signedAt := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err := DB.Exec(`UPDATE consent_forms SET signature_type = ?, signature_data = ?, signed_name = ?, signed_at = ?, status = 'signed'
		WHERE id = ?`, signatureType, signatureData, signedName, signedAt, f.ID)
	if err != nil {
		return err
	}
	return f.GetByID(f.ID)
}

func (f *ConsentForm) Revoke() error {
	_, err := DB.Exec(`UPDATE consent_forms SET status = 'revoked' WHERE id = ?`, f.ID)
	return err
}

func (f *ConsentForm) Delete() error {
	res, err := DB.Exec("DELETE FROM consent_forms WHERE id = ?", f.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
