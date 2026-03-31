package models

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
	CreatedAt  Date `json:"createdAt"`
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

func (e *AuditLogEntry) GetByUserName(userName string, limit int) (AuditLogEntryList, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := DB.Query(`SELECT `+auditLogEntryColumns+` FROM audit_log WHERE user_name = ? ORDER BY created_at DESC LIMIT ?`, userName, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list AuditLogEntryList
	list.ScanRows(rows)
	return list, nil
}

func (e *AuditLogEntry) GetAll(entityType, entityID, action string, limit int) (AuditLogEntryList, error) {
	query := `SELECT ` + auditLogEntryColumns + ` FROM audit_log WHERE 1=1`
	var args []interface{}

	if entityType != "" {
		query += " AND entity_type = ?"
		args = append(args, entityType)
	}
	if entityID != "" {
		query += " AND entity_id = ?"
		args = append(args, entityID)
	}
	if action != "" {
		query += " AND action = ?"
		args = append(args, action)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	} else {
		query += " LIMIT 200"
	}

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list AuditLogEntryList
	list.ScanRows(rows)
	return list, nil
}
