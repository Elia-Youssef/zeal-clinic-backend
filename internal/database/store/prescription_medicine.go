package store

import (
	"database/sql"
	"errors"
)

const prescriptionMedicineColumnsNoId = `medicine_id, prescription_id, instructions, created_at`
const prescriptionMedicineColumns = `id, ` + prescriptionMedicineColumnsNoId

type PrescriptionMedicine struct {
	ID             string `json:"id"`
	MedicineID     string `json:"medicineId"`
	PrescriptionID string `json:"prescriptionId"`
	Instructions   string `json:"instructions"`
	CreatedAt      Date   `json:"createdAt"`
	// Joined fields
	MedicineName string `json:"medicineName,omitempty"`
}

type PrescriptionMedicineList []PrescriptionMedicine

func (l *PrescriptionMedicineList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil PrescriptionMedicine rows")
	}
	*l = PrescriptionMedicineList{}
	for rows.Next() {
		var item PrescriptionMedicine
		err := rows.Scan(&item.ID, &item.MedicineID, &item.PrescriptionID, &item.Instructions, &item.CreatedAt, &item.MedicineName)
		if err != nil {
			return err
		}
		*l = append(*l, item)
	}
	return nil
}

func (pm *PrescriptionMedicineList) GetByPrescription(prescriptionID string) error {
	rows, err := RDB.Query(`SELECT pm.id, pm.medicine_id, pm.prescription_id, pm.instructions, pm.created_at,
		m.name
		FROM prescription_medicines pm
		JOIN medicines m ON m.id = pm.medicine_id
		WHERE pm.prescription_id = ? ORDER BY pm.created_at`, prescriptionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := pm.ScanRows(rows); err != nil {
		return err
	}
	return rows.Err()
}

// MedicinesByPatient loads every prescription medicine for a patient's
// prescriptions in one query, grouped by prescription ID, to avoid an N+1 over
// the patient's prescriptions.
func MedicinesByPatient(patientID string) (map[string]PrescriptionMedicineList, error) {
	rows, err := RDB.Query(`SELECT pm.id, pm.medicine_id, pm.prescription_id, pm.instructions, pm.created_at, m.name
		FROM prescription_medicines pm
		JOIN medicines m ON m.id = pm.medicine_id
		JOIN prescriptions p ON p.id = pm.prescription_id
		WHERE p.patient_id = ? ORDER BY pm.created_at`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	grouped := make(map[string]PrescriptionMedicineList)
	for rows.Next() {
		var item PrescriptionMedicine
		if err := rows.Scan(&item.ID, &item.MedicineID, &item.PrescriptionID, &item.Instructions, &item.CreatedAt, &item.MedicineName); err != nil {
			return nil, err
		}
		grouped[item.PrescriptionID] = append(grouped[item.PrescriptionID], item)
	}
	return grouped, rows.Err()
}
