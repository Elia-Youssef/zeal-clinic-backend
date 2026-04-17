package models

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type Voucher struct {
	ID         string `json:"id"`
	DiscountID string `json:"discountId"`
	Code       string `json:"code"`
	IsUsed     int    `json:"isUsed"`
	CreatedAt  Date   `json:"createdAt"`
}

const voucherColumnsNoId = `discount_id, code, is_used, created_at`
const voucherColumns = `id, ` + voucherColumnsNoId

type VoucherList []Voucher

func (v *Voucher) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Voucher row")
	}
	return row.Scan(&v.ID, &v.DiscountID, &v.Code, &v.IsUsed, &v.CreatedAt)
}

func (l *VoucherList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Voucher rows")
	}
	*l = VoucherList{}
	for rows.Next() {
		var item Voucher
		if err := rows.Scan(&item.ID, &item.DiscountID, &item.Code, &item.IsUsed, &item.CreatedAt); err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (l *VoucherList) GetAll(params ListParams, itemID string) (int, error) {
	from := " FROM vouchers v"
	where := " WHERE 1=1"
	var args []interface{}
	if itemID != "" {
		from += ` JOIN discounts d ON d.id = v.discount_id
			JOIN discount_items di ON di.discount_id = d.id`
		where += " AND di.item_id = ? AND d.is_active = 1"
		args = append(args, itemID)
	}
	if fc, fa := params.FilterClause("v.code"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(DISTINCT v.id)"+from+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	query := `SELECT DISTINCT v.id, v.discount_id, v.code, v.is_used, v.created_at` + from + where + ` ORDER BY v.created_at DESC` + params.PaginationClause()
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

func (l *VoucherList) GetByDiscount(discountID string) error {
	rows, err := RDB.Query(`SELECT `+voucherColumns+` FROM vouchers WHERE discount_id = ? ORDER BY created_at`, discountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	return l.ScanRows(rows)
}

func (v *Voucher) GetByID(id string) error {
	row := RDB.QueryRow(`SELECT `+voucherColumns+` FROM vouchers WHERE id = ?`, id)
	return v.ScanRow(row)
}

func (v *Voucher) GetByCode(code string) error {
	row := RDB.QueryRow(`SELECT `+voucherColumns+` FROM vouchers WHERE code = ?`, code)
	return v.ScanRow(row)
}

func (v *Voucher) Create() error {
	v.ID = uuid.Must(uuid.NewV7()).String()
	v.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO vouchers (`+voucherColumns+`) VALUES (?,?,?,?,?)`,
		v.ID, v.DiscountID, v.Code, v.IsUsed, v.CreatedAt)
	return err
}

func (v *Voucher) MarkUsed() error {
	_, err := DB.Exec(`UPDATE vouchers SET is_used = 1 WHERE id = ?`, v.ID)
	return err
}

func DeleteVoucher(id string) error {
	res, err := DB.Exec("DELETE FROM vouchers WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
