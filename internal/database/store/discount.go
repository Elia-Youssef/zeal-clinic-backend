package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Discount struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	DiscountType  string  `json:"discountType"`
	ValueType     string  `json:"valueType"`
	Value         float64 `json:"value"`
	MaxUsages     *int    `json:"maxUsages"`
	CurrentUsages int     `json:"currentUsages"`
	StartDate     *Date   `json:"startDate"`
	EndDate       *Date   `json:"endDate"`
	IsActive      int     `json:"isActive"`
	CreatedAt     Date    `json:"createdAt"`
	UpdatedAt     Date    `json:"updatedAt"`
	// Nested
	Items    DiscountItemList `json:"items,omitempty"`
	Vouchers VoucherList      `json:"vouchers,omitempty"`
}

func (d *Discount) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(d.Name, "Name"); msg != "" {
		e["name"] = msg
	}
	if msg := validation.Required(d.DiscountType, "Discount type"); msg != "" {
		e["discountType"] = msg
	} else if msg := validation.OneOf(d.DiscountType, []string{"offer", "voucher", "gift"}, "Discount type"); msg != "" {
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
	if len(e) > 0 {
		return e
	}
	return nil
}

const discountColumnsNoId = `name, description, discount_type, value_type, value, max_usages, current_usages, start_date, end_date, is_active, created_at, updated_at`
const discountColumns = `id, ` + discountColumnsNoId

type DiscountList []Discount

func (d *Discount) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Discount row")
	}
	return row.Scan(&d.ID, &d.Name, &d.Description, &d.DiscountType, &d.ValueType, &d.Value,
		&d.MaxUsages, &d.CurrentUsages, &d.StartDate, &d.EndDate,
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
			&item.MaxUsages, &item.CurrentUsages, &item.StartDate, &item.EndDate,
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

	query := `SELECT ` + discountColumns + ` FROM discounts` + where + ` ORDER BY created_at DESC` + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if err := l.ScanRows(rows); err != nil {
		return 0, err
	}
	for i := range *l {
		(*l)[i].Items.GetByDiscount((*l)[i].ID)
		(*l)[i].Vouchers.GetByDiscount((*l)[i].ID)
	}
	return total, rows.Err()
}

func (l *DiscountList) GetByItem(itemID string) error {
	query := `SELECT DISTINCT d.id, d.name, d.description, d.discount_type, d.value_type, d.value,
			d.max_usages, d.current_usages, d.start_date, d.end_date,
			d.is_active, d.created_at, d.updated_at
		FROM discounts d
		INNER JOIN discount_items di ON di.discount_id = d.id
		WHERE di.item_id = ? AND d.is_active = 1
		ORDER BY d.created_at DESC`
	rows, err := RDB.Query(query, itemID)
	if err != nil {
		return err
	}
	defer rows.Close()

	if err := l.ScanRows(rows); err != nil {
		return err
	}
	for i := range *l {
		(*l)[i].Items.GetByDiscount((*l)[i].ID)
		(*l)[i].Vouchers.GetByDiscount((*l)[i].ID)
	}
	return rows.Err()
}

func (d *Discount) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+discountColumns+` FROM discounts WHERE id = ?`, id)
	if err := d.ScanRow(row); err != nil {
		return err
	}
	d.Items.GetByDiscount(d.ID)
	d.Vouchers.GetByDiscount(d.ID)
	return nil
}

func (d *Discount) Create() error {
	d.ID = uuid.Must(uuid.NewV7()).String()
	now := DateNow()
	d.CreatedAt = now
	d.UpdatedAt = now
	if d.IsActive == 0 {
		d.IsActive = 1
	}

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO discounts (`+discountColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		d.ID, d.Name, d.Description, d.DiscountType, d.ValueType, d.Value,
		d.MaxUsages, d.CurrentUsages, d.StartDate, d.EndDate,
		d.IsActive, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return err
	}

	for i := range d.Items {
		d.Items[i].ID = uuid.Must(uuid.NewV7()).String()
		d.Items[i].DiscountID = d.ID
		d.Items[i].CreatedAt = now
		_, err = tx.Exec(`INSERT INTO discount_items (`+discountItemColumns+`) VALUES (?,?,?,?,?)`,
			d.Items[i].ID, d.Items[i].DiscountID, d.Items[i].ItemType, d.Items[i].ItemID, d.Items[i].CreatedAt)
		if err != nil {
			return err
		}
	}

	for i := range d.Vouchers {
		d.Vouchers[i].ID = uuid.Must(uuid.NewV7()).String()
		d.Vouchers[i].DiscountID = d.ID
		d.Vouchers[i].CreatedAt = now
		_, err = tx.Exec(`INSERT INTO vouchers (`+voucherColumns+`) VALUES (?,?,?,?,?)`,
			d.Vouchers[i].ID, d.Vouchers[i].DiscountID, d.Vouchers[i].Code, d.Vouchers[i].IsUsed, d.Vouchers[i].CreatedAt)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *Discount) Update(updates map[string]any) error {
	cols := map[string]string{
		"name": "name", "description": "description", "discountType": "discount_type",
		"valueType": "value_type", "value": "value",
		"maxUsages": "max_usages", "startDate": "start_date", "endDate": "end_date",
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

func (d *Discount) IncrementUsage() error {
	_, err := DB.Exec(`UPDATE discounts SET current_usages = current_usages + 1, updated_at = ? WHERE id = ?`, DateNow(), d.ID)
	return err
}

// Discount Items

type DiscountItem struct {
	ID         string `json:"id"`
	DiscountID string `json:"discountId"`
	ItemType   string `json:"itemType"`
	ItemID     string `json:"itemId"`
	CreatedAt  Date   `json:"createdAt"`
	// Transient
	ItemName string `json:"itemName"`
}

const discountItemColumnsNoId = `discount_id, item_type, item_id, created_at`
const discountItemColumns = `id, ` + discountItemColumnsNoId

type DiscountItemList []DiscountItem

func (l *DiscountItemList) GetByDiscount(discountID string) error {
	rows, err := RDB.Query(`SELECT `+discountItemColumns+` FROM discount_items WHERE discount_id = ? ORDER BY created_at`, discountID)
	if err != nil {
		return err
	}
	defer rows.Close()

	*l = DiscountItemList{}
	for rows.Next() {
		var item DiscountItem
		if err := rows.Scan(&item.ID, &item.DiscountID, &item.ItemType, &item.ItemID, &item.CreatedAt); err != nil {
			continue
		}
		switch item.ItemType {
		case "procedure":
			RDB.QueryRow(`SELECT name FROM procedures WHERE id = ?`, item.ItemID).Scan(&item.ItemName)
		case "product":
			RDB.QueryRow(`SELECT name FROM products WHERE id = ?`, item.ItemID).Scan(&item.ItemName)
		}
		*l = append(*l, item)
	}
	return nil
}

func CreateDiscountItem(discountID string, item *DiscountItem) error {
	item.ID = uuid.Must(uuid.NewV7()).String()
	item.DiscountID = discountID
	item.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO discount_items (`+discountItemColumns+`) VALUES (?,?,?,?,?)`,
		item.ID, item.DiscountID, item.ItemType, item.ItemID, item.CreatedAt)
	return err
}

func DeleteDiscountItem(id string) error {
	res, err := DB.Exec("DELETE FROM discount_items WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
