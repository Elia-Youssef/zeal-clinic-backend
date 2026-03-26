package models

type Room struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	IsAvailable bool   `json:"isAvailable"`
	CreatedAt   string `json:"createdAt"`
}

func (r *Room) GetAll() ([]Room, error) {
	rows, err := DB.Query("SELECT id, name, type, is_available, created_at FROM rooms ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var rm Room
		var avail int
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.Type, &avail, &rm.CreatedAt); err != nil {
			return nil, err
		}
		rm.IsAvailable = avail == 1
		rooms = append(rooms, rm)
	}
	return rooms, rows.Err()
}
