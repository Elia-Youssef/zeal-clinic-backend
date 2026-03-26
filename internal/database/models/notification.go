package models

import "database/sql"

type Notification struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	BookingID string `json:"bookingId"`
	IsRead    bool   `json:"isRead"`
	CreatedAt string `json:"createdAt"`
}

func (n *Notification) GetAll(unreadOnly bool) ([]Notification, error) {
	query := `SELECT id, type, title, message, booking_id, is_read, created_at FROM notifications`
	if unreadOnly {
		query += " WHERE is_read = 0"
	}
	query += " ORDER BY created_at DESC"

	rows, err := DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		var isRead int
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Message, &n.BookingID, &isRead, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.IsRead = isRead == 1
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func (n *Notification) GetUnreadCount() (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM notifications WHERE is_read = 0`).Scan(&count)
	return count, err
}

func (n *Notification) MarkRead() error {
	res, err := DB.Exec(`UPDATE notifications SET is_read = 1 WHERE id = ?`, n.ID)
	if err != nil {
		return err
	}
	num, _ := res.RowsAffected()
	if num == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (n *Notification) MarkAllRead() error {
	_, err := DB.Exec(`UPDATE notifications SET is_read = 1 WHERE is_read = 0`)
	return err
}
