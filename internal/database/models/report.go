package models

type Report struct{}

type ForecastResult struct {
	ThisWeek   float64            `json:"thisWeek"`
	ThisMonth  float64            `json:"thisMonth"`
	NextMonth  float64            `json:"nextMonth"`
	ByCategory []CategoryForecast `json:"byCategory"`
}

type CategoryForecast struct {
	Category string  `json:"category"`
	Count    int     `json:"count"`
	Revenue  float64 `json:"revenue"`
}

type ProfitLossResult struct {
	TotalRevenue     float64                    `json:"totalRevenue"`
	TotalExpenses    float64                    `json:"totalExpenses"`
	NetProfit        float64                    `json:"netProfit"`
	ExpenseBreakdown []ExpenseCategoryBreakdown `json:"expenseBreakdown"`
}

type ExpenseCategoryBreakdown struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
}

func (r *Report) GetForecast() (ForecastResult, error) {
	var result ForecastResult

	rows, err := DB.Query(`
		SELECT treatment_type, COUNT(*) as cnt
		FROM appointments
		WHERE status = 'Scheduled'
		AND date(start_time) >= date('now')
		AND date(start_time) < date('now', '+7 days')
		GROUP BY treatment_type
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cat CategoryForecast
			if err := rows.Scan(&cat.Category, &cat.Count); err == nil {
				result.ByCategory = append(result.ByCategory, cat)
			}
		}
	}

	_ = DB.QueryRow(`
		SELECT COUNT(*) FROM appointments
		WHERE status = 'Scheduled'
		AND strftime('%%Y-%%m', start_time) = strftime('%%Y-%%m', 'now')
	`).Scan(&result.ThisMonth)

	_ = DB.QueryRow(`
		SELECT COUNT(*) FROM appointments
		WHERE status = 'Scheduled'
		AND strftime('%%Y-%%m', start_time) = strftime('%%Y-%%m', 'now', '+1 month')
	`).Scan(&result.NextMonth)

	if result.ByCategory == nil {
		result.ByCategory = []CategoryForecast{}
	}

	return result, nil
}

func (r *Report) GetProfitLoss(from, to string) (ProfitLossResult, error) {
	var result ProfitLossResult

	_ = DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0) FROM invoices
		WHERE type IN ('invoice','receipt') AND status = 'paid'
		AND date(created_at) >= date(?) AND date(created_at) <= date(?)
	`, from, to).Scan(&result.TotalRevenue)

	_ = DB.QueryRow(`
		SELECT COALESCE(SUM(amount), 0) FROM invoices
		WHERE type = 'expense'
		AND date(created_at) >= date(?) AND date(created_at) <= date(?)
	`, from, to).Scan(&result.TotalExpenses)

	if result.ExpenseBreakdown == nil {
		result.ExpenseBreakdown = []ExpenseCategoryBreakdown{}
	}

	result.NetProfit = result.TotalRevenue - result.TotalExpenses
	return result, nil
}
