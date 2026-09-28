package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Notification struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Action      string `json:"action"`
	IsRead      bool   `json:"isRead"`
	CreatedAt   Date   `json:"createdAt"`
}

const notificationColumnsNoId = `user_id, title, description, action, is_read, created_at`
const notificationColumns = `id, ` + notificationColumnsNoId

type NotificationList []Notification

func (m *Notification) ScanRow(row *sql.Row) error {
	if row == nil {
		return errors.New("nil Notification row")
	}
	var isReadRaw int
	err := row.Scan(&m.ID, &m.UserID, &m.Title, &m.Description, &m.Action, &isReadRaw, &m.CreatedAt)
	if err != nil {
		return err
	}
	m.IsRead = isReadRaw == 1
	return nil
}

func (l *NotificationList) ScanRows(rows *sql.Rows) error {
	if rows == nil {
		return errors.New("nil Notification rows")
	}
	*l = NotificationList{}
	for rows.Next() {
		var item Notification
		var isReadRaw int
		err := rows.Scan(&item.ID, &item.UserID, &item.Title, &item.Description, &item.Action, &isReadRaw, &item.CreatedAt)
		if err != nil {
			continue
		}
		item.IsRead = isReadRaw == 1
		*l = append(*l, item)
	}
	return nil
}

func (n *Notification) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(n.UserID, "User ID"); msg != "" {
		e["userId"] = msg
	}
	if msg := validation.Required(n.Title, "Title"); msg != "" {
		e["title"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

func (nl *NotificationList) GetAll(userID string, params ListParams) (int, error) {
	where := " WHERE user_id = ?"
	args := []any{userID}

	if fc, fa := params.FilterClause("title", "description"); fc != "" {
		where += " AND " + fc
		args = append(args, fa...)
	}

	var total int
	if err := RDB.QueryRow("SELECT COUNT(*) FROM notifications"+where, args...).Scan(&total); err != nil {
		return 0, err
	}

	order := params.OrderClause(map[string]string{
		"title":       "title",
		"description": "description",
		"action":      "action",
		"isRead":      "is_read",
		"createdAt":   "created_at",
	}, "created_at DESC")
	query := `SELECT ` + notificationColumns + ` FROM notifications` + where + order + params.PaginationClause()
	rows, err := RDB.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	nl.ScanRows(rows)
	return total, nil
}

func (n *Notification) GetByID(id string) error {
	return n.ScanRow(RDB.QueryRow(`SELECT `+notificationColumns+` FROM notifications WHERE id = ?`, id))
}

func (n *Notification) Create() error {
	n.ID = uuid.Must(uuid.NewV7()).String()
	n.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO notifications (`+notificationColumns+`) VALUES (?,?,?,?,?,?,?)`,
		n.ID, n.UserID, n.Title, n.Description, n.Action, BoolToInt(n.IsRead), n.CreatedAt)
	return err
}

func MarkNotificationRead(id string) error {
	res, err := DB.Exec("UPDATE notifications SET is_read = 1 WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func MarkAllNotificationsRead(userID string) error {
	_, err := DB.Exec("UPDATE notifications SET is_read = 1 WHERE user_id = ? AND is_read = 0", userID)
	return err
}

func GetUnreadCount(userID string) (int, error) {
	var count int
	err := RDB.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0", userID).Scan(&count)
	return count, err
}

func (n *Notification) Delete() error {
	res, err := DB.Exec("DELETE FROM notifications WHERE id = ?", n.ID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteNotificationsByAction deletes every notification carrying action and
// returns the user each deleted row belonged to, one entry per row.
func DeleteNotificationsByAction(action string) ([]string, error) {
	rows, err := DB.Query(`DELETE FROM notifications WHERE action = ? RETURNING user_id`, action)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, rows.Err()
}

func GetUserIDForNotification(id string) (string, error) {
	var userID string
	err := RDB.QueryRow("SELECT user_id FROM notifications WHERE id = ?", id).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("notification not found")
	}
	return userID, nil
}
