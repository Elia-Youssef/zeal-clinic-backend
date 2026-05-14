package store

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type AuditLogEntry struct {
	ID         string `json:"id"`
	UserName   string `json:"userName"`
	UserRole   string `json:"userRole"`
	Action     string `json:"action"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Details    string `json:"details"`
	IPAddress  string `json:"ipAddress"`
	CreatedAt  Date   `json:"createdAt"`
}

const auditLogEntryColumnsNoId = `user_name, user_role, action, entity_type, entity_id, details, ip_address, created_at`
const auditLogEntryColumns = `id, ` + auditLogEntryColumnsNoId

type AuditLogEntryList []AuditLogEntry

func (m *AuditLogEntry) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil AuditLogEntry row")
	}
	return row.Scan(&m.ID, &m.UserName, &m.UserRole, &m.Action, &m.EntityType, &m.EntityID, &m.Details, &m.IPAddress, &m.CreatedAt)
}

func (l *AuditLogEntryList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil AuditLogEntry rows")
	}
	*l = AuditLogEntryList{}
	for rows.Next() {
		var item AuditLogEntry
		err := rows.Scan(&item.ID, &item.UserName, &item.UserRole, &item.Action, &item.EntityType, &item.EntityID, &item.Details, &item.IPAddress, &item.CreatedAt)
		if err != nil {
			continue
		}
		*l = append(*l, item)
	}
	return nil
}

func (e *AuditLogEntry) Log() error {
	e.ID = uuid.Must(uuid.NewV7()).String()
	if e.CreatedAt == "" {
		e.CreatedAt = DateNow()
	}

	_, err := DB.Exec(`INSERT INTO audit_log (`+auditLogEntryColumns+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		e.ID, e.UserName, e.UserRole, e.Action, e.EntityType, e.EntityID, e.Details, e.IPAddress, e.CreatedAt,
	)
	return err
}

func (e *AuditLogEntryList) GetByUserName(userName string, params ListParams) (int, error) {
	where := " WHERE user_name = ?"
	args := []any{userName}
	if fc, fa := params.FilterClause("action", "details", "entity_type"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM audit_log"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"userName":   "user_name",
		"userRole":   "user_role",
		"action":     "action",
		"entityType": "entity_type",
		"entityId":   "entity_id",
		"createdAt":  "created_at",
	}, "created_at DESC")
	query := `SELECT ` + auditLogEntryColumns + ` FROM audit_log` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	e.ScanRows(rows)
	return total, nil
}

func (e *AuditLogEntryList) GetAll(entityType, entityID, action string, params ListParams) (int, error) {
	where := " WHERE 1=1"
	var args []any

	if entityType != "" {
		where += " AND entity_type = ?"
		args = append(args, entityType)
	}
	if entityID != "" {
		where += " AND entity_id = ?"
		args = append(args, entityID)
	}
	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
	}
	if fc, fa := params.FilterClause("user_name", "details"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM audit_log"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"userName":   "user_name",
		"userRole":   "user_role",
		"action":     "action",
		"entityType": "entity_type",
		"entityId":   "entity_id",
		"createdAt":  "created_at",
	}, "created_at DESC")
	query := `SELECT ` + auditLogEntryColumns + ` FROM audit_log` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	e.ScanRows(rows)
	return total, nil
}
