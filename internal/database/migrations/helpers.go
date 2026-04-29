package migrations

import "embed"

//go:embed data/countries.json
//go:embed data/lebanon_cities.json
var dataFS embed.FS

// FS exposes the SQL migration files for the goose runner.
//
//go:embed *.sql
var FS embed.FS
