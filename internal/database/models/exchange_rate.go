package models

import (
	"time"

	"github.com/google/uuid"
)

type ExchangeRate struct {
	ID            string  `json:"id"`
	FromCurrency  string  `json:"fromCurrency"`
	ToCurrency    string  `json:"toCurrency"`
	Rate          float64 `json:"rate"`
	SetBy         string  `json:"setBy"`
	EffectiveDate string  `json:"effectiveDate"`
	CreatedAt     string  `json:"createdAt"`
}

func (r *ExchangeRate) GetLatest(from, to string) error {
	err := DB.QueryRow(`SELECT id, from_currency, to_currency, rate, set_by, effective_date, created_at
		FROM exchange_rates WHERE from_currency = ? AND to_currency = ?
		ORDER BY effective_date DESC, created_at DESC LIMIT 1`, from, to).Scan(
		&r.ID, &r.FromCurrency, &r.ToCurrency, &r.Rate, &r.SetBy, &r.EffectiveDate, &r.CreatedAt,
	)
	return err
}

func (r *ExchangeRate) GetAll() ([]ExchangeRate, error) {
	rows, err := DB.Query(`SELECT id, from_currency, to_currency, rate, set_by, effective_date, created_at
		FROM exchange_rates ORDER BY effective_date DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rates []ExchangeRate
	for rows.Next() {
		var rate ExchangeRate
		if err := rows.Scan(&rate.ID, &rate.FromCurrency, &rate.ToCurrency, &rate.Rate, &rate.SetBy, &rate.EffectiveDate, &rate.CreatedAt); err != nil {
			return nil, err
		}
		rates = append(rates, rate)
	}
	return rates, rows.Err()
}

func (r *ExchangeRate) Create() error {
	r.ID = uuid.New().String()
	r.CreatedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
	if r.FromCurrency == "" {
		r.FromCurrency = "USD"
	}
	if r.ToCurrency == "" {
		r.ToCurrency = "LBP"
	}
	if r.EffectiveDate == "" {
		r.EffectiveDate = time.Now().Format("2006-01-02")
	}

	_, err := DB.Exec(`INSERT INTO exchange_rates (id, from_currency, to_currency, rate, set_by, effective_date, created_at)
		VALUES (?,?,?,?,?,?,?)`, r.ID, r.FromCurrency, r.ToCurrency, r.Rate, r.SetBy, r.EffectiveDate, r.CreatedAt)
	return err
}
