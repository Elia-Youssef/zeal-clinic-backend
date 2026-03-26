package models

import (
	"time"

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
	CreatedAt  string `json:"createdAt"`
}

func (e *AuditLogEntry) Log() error {
	e.ID = uuid.New().String()
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	}

	_, err := DB.Exec(`INSERT INTO audit_log (id, user_name, user_role, action, entity_type, entity_id, details, ip_address, created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		e.ID, e.UserName, e.UserRole, e.Action, e.EntityType, e.EntityID, e.Details, e.IPAddress, e.CreatedAt,
	)
	return err
}

func (e *AuditLogEntry) GetAll(entityType, entityID, action string, limit int) ([]AuditLogEntry, error) {
	query := `SELECT id, user_name, user_role, action, entity_type, entity_id, details, ip_address, created_at
		FROM audit_log WHERE 1=1`
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

	var entries []AuditLogEntry
	for rows.Next() {
		var entry AuditLogEntry
		if err := rows.Scan(&entry.ID, &entry.UserName, &entry.UserRole, &entry.Action, &entry.EntityType, &entry.EntityID, &entry.Details, &entry.IPAddress, &entry.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}
