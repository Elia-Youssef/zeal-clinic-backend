package httpx

type Response struct {
	Error   string
	Success bool
	Data    any
}

type PaginatedList struct {
	Items any `json:"items"`
	Total int `json:"total"`
}
