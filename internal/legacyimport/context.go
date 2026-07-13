package legacyimport

import (
	"path/filepath"

	"clinic-api/internal/database/store"
	"clinic-api/internal/legacyimport/csvutil"
	"clinic-api/internal/legacyimport/idgen"
)

const (
	usdCurrency = store.USDCurrencyID

	migrationFallbackTS = "2024-01-01T00:00:00Z"

	fallbackRoomName = "Unassigned"
)

var seedRoomNames = map[string]bool{
	"Room 1": true, "Room 2": true, "Room 3": true, "Room 4": true,
	"Room 5": true, "Room 6": true, "Room 7": true, "Hospital": true,
}

type Context struct {
	InputDir string
	Report   *Report

	MigrationTS string
	baseMillis  int64

	files map[string]*csvutil.File

	selfBalanceID  string
	roomID         map[string]string
	fallbackRoomID string
	procTypeID     map[string]string
	procByName     map[string]string
	procByCategory map[string]string

	knownPatient map[string]bool
	patientName  map[string]string
	backfillIDs  []string
	backfillName map[string]string

	financialPatients []string
	financialSeen     map[string]bool
}

func newContext(inputDir string) *Context {
	return &Context{
		InputDir:       inputDir,
		Report:         newReport(),
		MigrationTS:    migrationFallbackTS,
		baseMillis:     idgen.MillisFromZ(migrationFallbackTS, 0),
		files:          map[string]*csvutil.File{},
		roomID:         map[string]string{},
		procTypeID:     map[string]string{},
		procByName:     map[string]string{},
		procByCategory: map[string]string{},
		knownPatient:   map[string]bool{},
		patientName:    map[string]string{},
		backfillName:   map[string]string{},
		financialSeen:  map[string]bool{},
	}
}

func (c *Context) ID(createdAtZ, key string) string {
	millis := c.baseMillis
	if createdAtZ != "" {
		millis = idgen.MillisFromZ(createdAtZ, c.baseMillis)
	}
	return idgen.UUIDv7(millis, key)
}

func (c *Context) patientBalanceID(patientID string) string {
	return store.DeterministicID("balances", "patient", patientID, usdCurrency)
}

func (c *Context) resolveRoomID(name string) string {
	if id, ok := c.roomID[name]; ok && id != "" {
		return id
	}
	return c.fallbackRoomID
}

func (c *Context) resolveProcedureID(name string) string {
	return c.procByName[name]
}

func (c *Context) resolveCategoryProcedureID(category string) string {
	return c.procByCategory[category]
}

func (c *Context) File(name string) (*csvutil.File, error) {
	if f, ok := c.files[name]; ok {
		return f, nil
	}
	f, err := csvutil.Load(filepath.Join(c.InputDir, name))
	if err != nil {
		return nil, err
	}
	c.files[name] = f
	return f, nil
}

func (c *Context) RegisterPatientRef(id, name string) bool {
	if id == "" || id == "0" {
		return false
	}
	if c.patientName[id] == "" && name != "" {
		c.patientName[id] = name
	}
	if !c.knownPatient[id] {
		if _, seen := c.backfillName[id]; !seen {
			c.backfillIDs = append(c.backfillIDs, id)
			c.backfillName[id] = name
		}
	}
	return true
}

func (c *Context) MarkFinancialPatient(id string) {
	if id == "" || id == "0" || c.financialSeen[id] {
		return
	}
	c.financialSeen[id] = true
	c.financialPatients = append(c.financialPatients, id)
}

func normalizeCurrency(raw string) string {
	switch raw {
	case "", "LBP", "lbp", "USD", "usd", "$":
		return usdCurrency
	default:
		return raw
	}
}
