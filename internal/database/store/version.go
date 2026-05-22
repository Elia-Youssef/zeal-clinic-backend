package store

import (
	"clinic-api/internal/validation"
	"database/sql"
	"errors"

	"github.com/google/uuid"
)

// Version is an advertised app build, published on cloud and synced to locals.
type Version struct {
	ID        string `json:"id"`
	Version   string `json:"version"`
	Platform  string `json:"platform"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Notes     string `json:"notes"`
	CreatedAt Date   `json:"createdAt"`
}

func (v *Version) IsValid() error {
	e := make(validation.Errors)
	if msg := validation.Required(v.Version, "Version"); msg != "" {
		e["version"] = msg
	}
	if v.Platform != "windows" && v.Platform != "linux" {
		e["platform"] = "Platform must be 'windows' or 'linux'"
	}
	if msg := validation.Required(v.URL, "URL"); msg != "" {
		e["url"] = msg
	}
	if len(e) > 0 {
		return e
	}
	return nil
}

const versionColumns = `id, version, platform, url, sha256, notes, created_at`

func (v *Version) scan(s interface{ Scan(...any) error }) error {
	return s.Scan(&v.ID, &v.Version, &v.Platform, &v.URL, &v.SHA256, &v.Notes, &v.CreatedAt)
}

// LatestVersion returns the most recently published build for a platform (newest
// row wins, not highest semver, so publishing an older version rolls back).
// Returns a zero Version when none exist.
func LatestVersion(platform string) (Version, error) {
	var v Version
	err := v.scan(RDB.QueryRow(
		`SELECT `+versionColumns+` FROM versions WHERE platform = ? ORDER BY created_at DESC, id DESC LIMIT 1`,
		platform))
	if errors.Is(err, sql.ErrNoRows) {
		return Version{}, nil
	}
	return v, err
}

func (v *Version) Create() error {
	if v.ID == "" {
		v.ID = uuid.Must(uuid.NewV7()).String()
	}
	v.CreatedAt = DateNow()
	_, err := DB.Exec(`INSERT INTO versions (`+versionColumns+`) VALUES (?,?,?,?,?,?,?)`,
		v.ID, v.Version, v.Platform, v.URL, v.SHA256, v.Notes, v.CreatedAt)
	return err
}

// AppState is this machine's single-row self-update state (never synced).
type AppState struct {
	Installing    bool   `json:"installing"`
	TargetVersion string `json:"targetVersion"`
	UpdatedAt     Date   `json:"updatedAt"`
}

func GetAppState() (AppState, error) {
	var s AppState
	var installing int
	err := RDB.QueryRow(`SELECT installing, target_version, updated_at FROM app_state WHERE id = 1`).
		Scan(&installing, &s.TargetVersion, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AppState{}, nil
	}
	s.Installing = installing != 0
	return s, err
}

func SetInstalling(target string) error {
	_, err := DB.Exec(
		`UPDATE app_state SET installing = 1, target_version = ?, updated_at = ? WHERE id = 1`,
		target, DateNow())
	return err
}

func ClearInstalling() error {
	_, err := DB.Exec(
		`UPDATE app_state SET installing = 0, updated_at = ? WHERE id = 1`, DateNow())
	return err
}
