package store

import (
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

type AuditLogEntry struct {
	ID         string `json:"id"`
	UserID     string `json:"userId"`
	Username   string `json:"username"`
	UserRole   string `json:"userRole"`
	Action     string `json:"action"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	Details    string `json:"details"`
	IPAddress  string `json:"ipAddress"`
	CreatedAt  Date   `json:"createdAt"`
}

const auditLogEntryColumnsNoId = `user_id, user_role, action, entity_type, entity_id, details, ip_address, created_at`
const auditLogEntryColumns = `id, ` + auditLogEntryColumnsNoId

const auditLogSelectColumns = `a.id, a.user_id, COALESCE(u.username, ''), a.user_role, a.action, a.entity_type, a.entity_id, a.details, a.ip_address, a.created_at`
const auditLogFromJoin = ` FROM audit_log a LEFT JOIN users u ON u.id = a.user_id`

type AuditLogEntryList []AuditLogEntry

func (m *AuditLogEntry) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil AuditLogEntry row")
	}
	return row.Scan(&m.ID, &m.UserID, &m.Username, &m.UserRole, &m.Action, &m.EntityType, &m.EntityID, &m.Details, &m.IPAddress, &m.CreatedAt)
}

func (l *AuditLogEntryList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil AuditLogEntry rows")
	}
	*l = AuditLogEntryList{}
	for rows.Next() {
		var item AuditLogEntry
		err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.UserRole, &item.Action, &item.EntityType, &item.EntityID, &item.Details, &item.IPAddress, &item.CreatedAt)
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
		e.ID, e.UserID, e.UserRole, e.Action, e.EntityType, e.EntityID, e.Details, e.IPAddress, e.CreatedAt,
	)
	return err
}

func (e *AuditLogEntryList) GetByUserID(userID string, params ListParams) (int, error) {
	where := " WHERE a.user_id = ?"
	args := []any{userID}
	if fc, fa := params.FilterClause("a.action", "a.details", "a.entity_type"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*)"+auditLogFromJoin+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"username":   "u.username",
		"userRole":   "a.user_role",
		"action":     "a.action",
		"entityType": "a.entity_type",
		"entityId":   "a.entity_id",
		"createdAt":  "a.created_at",
	}, "a.created_at DESC")
	query := `SELECT ` + auditLogSelectColumns + auditLogFromJoin + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	e.ScanRows(rows)
	return total, nil
}

func (e *AuditLogEntryList) GetAll(entityType, entityID, action, userID string, params ListParams) (int, error) {
	where := " WHERE 1=1"
	var args []any

	if entityType != "" {
		where += " AND a.entity_type = ?"
		args = append(args, entityType)
	}
	if entityID != "" {
		where += " AND a.entity_id = ?"
		args = append(args, entityID)
	}
	if action != "" {
		where += " AND a.action = ?"
		args = append(args, action)
	}
	if userID != "" {
		where += " AND a.user_id = ?"
		args = append(args, userID)
	}
	if fc, fa := params.FilterClause("a.details", "u.username"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*)"+auditLogFromJoin+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"username":   "u.username",
		"userRole":   "a.user_role",
		"action":     "a.action",
		"entityType": "a.entity_type",
		"entityId":   "a.entity_id",
		"createdAt":  "a.created_at",
	}, "a.created_at DESC")
	query := `SELECT ` + auditLogSelectColumns + auditLogFromJoin + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	e.ScanRows(rows)
	return total, nil
}
