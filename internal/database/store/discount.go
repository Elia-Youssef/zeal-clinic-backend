package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Discount struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	DiscountType string  `json:"discountType"`
	ValueType    string  `json:"valueType"`
	Value        float64 `json:"value"`
	// Gift-only: at most one of PatientID / Code is set. PatientID means the
	// gift was assigned to a specific patient on creation (credit applied
	// immediately). Code means the gift is a redeemable code applied later.
	PatientID  *string `json:"patientId,omitempty"`
	Code       *string `json:"code,omitempty"`
	RedeemedAt *Date   `json:"redeemedAt,omitempty"`
	StartDate  *Date   `json:"startDate"`
	EndDate    *Date   `json:"endDate"`
	IsActive   int     `json:"isActive"`
	CreatedAt  Date    `json:"createdAt"`
	UpdatedAt  Date    `json:"updatedAt"`
}

func (d *Discount) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(d.Name, "Name"); msg != "" {
		e["name"] = msg
	}
	if msg := validation.Required(d.DiscountType, "Discount type"); msg != "" {
		e["discountType"] = msg
	} else if msg := validation.OneOf(d.DiscountType, []string{"offer", "gift"}, "Discount type"); msg != "" {
		e["discountType"] = msg
	}
	if msg := validation.Required(d.ValueType, "Value type"); msg != "" {
		e["valueType"] = msg
	} else if msg := validation.OneOf(d.ValueType, []string{"percentage", "fixed"}, "Value type"); msg != "" {
		e["valueType"] = msg
	}
	if msg := validation.Positive(d.Value, "Value"); msg != "" {
		e["value"] = msg
	}

	hasPatient := d.PatientID != nil && *d.PatientID != ""
	hasCode := d.Code != nil && *d.Code != ""
	switch d.DiscountType {
	case "gift":
		if hasPatient == hasCode {
			e["gift"] = "Gift must have exactly one of patientId or code"
		}
	case "offer":
		if hasPatient || hasCode {
			e["gift"] = "Only gift discounts can have patientId or code"
		}
	}

	if len(e) > 0 {
		return e
	}
	return nil
}

const discountColumnsNoId = `name, description, discount_type, value_type, value, patient_id, code, redeemed_at, start_date, end_date, is_active, created_at, updated_at`
const discountColumns = `id, ` + discountColumnsNoId

type DiscountList []Discount

func (d *Discount) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Discount row")
	}
	return row.Scan(&d.ID, &d.Name, &d.Description, &d.DiscountType, &d.ValueType, &d.Value,
		&d.PatientID, &d.Code, &d.RedeemedAt, &d.StartDate, &d.EndDate,
		&d.IsActive, &d.CreatedAt, &d.UpdatedAt)
}

func (l *DiscountList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Discount rows")
	}
	*l = DiscountList{}
	for rows.Next() {
		var item Discount
		err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.DiscountType, &item.ValueType, &item.Value,
			&item.PatientID, &item.Code, &item.RedeemedAt, &item.StartDate, &item.EndDate,
			&item.IsActive, &item.CreatedAt, &item.UpdatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (l *DiscountList) GetAll(params ListParams) (int, error) {
	where := ""
	var args []any
	if fc, fa := params.FilterClause("name", "description"); fc != "" {
		where = " WHERE " + fc
		args = fa
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM discounts"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"name":         "name",
		"description":  "description",
		"discountType": "discount_type",
		"valueType":    "value_type",
		"value":        "value",
		"startDate":    "start_date",
		"endDate":      "end_date",
		"isActive":     "is_active",
		"createdAt":    "created_at",
		"updatedAt":    "updated_at",
	}, "created_at DESC")
	query := `SELECT ` + discountColumns + ` FROM discounts` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := l.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, rows.Err()
}

func (d *Discount) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+discountColumns+` FROM discounts WHERE id = ?`, id)
	return d.ScanRow(row)
}

func (d *Discount) GetByCode(code string) error {
	row := RDB.QueryRow(`SELECT `+discountColumns+` FROM discounts WHERE code = ?`, code)
	return d.ScanRow(row)
}

func (d *Discount) Create() error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := d.CreateWithTx(tx); err != nil {
		return err
	}

	return tx.Commit()
}

func (d *Discount) CreateWithTx(tx *sql.Tx) error {
	d.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	d.CreatedAt = now
	d.UpdatedAt = now
	if d.IsActive == 0 {
		d.IsActive = 1
	}

	if _, err := tx.Exec(`INSERT INTO discounts (`+discountColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.Name, d.Description, d.DiscountType, d.ValueType, d.Value,
		d.PatientID, d.Code, d.RedeemedAt, d.StartDate, d.EndDate,
		d.IsActive, d.CreatedAt, d.UpdatedAt); err != nil {
		return err
	}

	return nil
}

func (d *Discount) Update(updates map[string]any) error {
	cols := map[string]string{
		"name": "name", "description": "description",
		"valueType": "value_type", "value": "value",
		"startDate": "start_date", "endDate": "end_date",
		"isActive": "is_active",
	}

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
		return d.GetByID(d.ID)
	}

	setClauses += ", updated_at = ?"
	args = append(args, DateNow())
	args = append(args, d.ID)
	if _, err := DB.Exec("UPDATE discounts SET "+setClauses+" WHERE id = ?", args...); err != nil {
		return err
	}
	return d.GetByID(d.ID)
}

func (d *Discount) Delete() error {
	res, err := DB.Exec("DELETE FROM discounts WHERE id = ?", d.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkRedeemed sets redeemed_at on the discount within the given tx.
func (d *Discount) MarkRedeemedWithTx(tx *sql.Tx) error {
	now := DateNow()
	if _, err := tx.Exec(`UPDATE discounts SET redeemed_at = ?, updated_at = ? WHERE id = ?`, now, now, d.ID); err != nil {
		return err
	}
	d.RedeemedAt = &now
	d.UpdatedAt = now
	return nil
}
