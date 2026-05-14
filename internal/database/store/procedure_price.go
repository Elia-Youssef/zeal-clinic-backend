package store

import (
	"database/sql"
	"errors"
)

type ProcedurePrice struct {
	ID          string  `json:"id"`
	ProcedureID string  `json:"procedureId"`
	Price       float64 `json:"price"`
	IsActive    bool    `json:"isActive"`
	CreatedAt   Date    `json:"createdAt"`
}

const procedurePriceColumns = `id, procedure_id, price, is_active, created_at`

type ProcedurePriceList []ProcedurePrice

func (m *ProcedurePrice) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil ProcedurePrice row")
	}
	var isActive int
	err := row.Scan(&m.ID, &m.ProcedureID, &m.Price, &isActive, &m.CreatedAt)
	if err != nil {
		return err
	}
	m.IsActive = isActive == 1
	return nil
}

func (l *ProcedurePriceList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil ProcedurePrice rows")
	}
	*l = ProcedurePriceList{}
	for rows.Next() {
		var item ProcedurePrice
		var isActive int
		err := rows.Scan(&item.ID, &item.ProcedureID, &item.Price, &isActive, &item.CreatedAt)
		if err != nil {
			continue
		}
		item.IsActive = isActive == 1
		*l = append(*l, item)
	}
	return nil
}

func (l *ProcedurePriceList) GetByProcedure(procedureID string, params ListParams) (int, error) {
	var total int
	if err := RDB.QueryRow(`SELECT COUNT(*) FROM procedure_prices WHERE procedure_id = ?`, procedureID).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"price":     "price",
		"isActive":  "is_active",
		"createdAt": "created_at",
	}, "is_active DESC, created_at DESC")
	rows, err := RDB.Query(`SELECT `+procedurePriceColumns+` FROM procedure_prices WHERE procedure_id = ?`+order+params.PaginationClause(), procedureID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if err := l.ScanRows(rows); err != nil {
		return 0, err
	}
	return total, nil
}
