package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"clinic-api/internal/auth"
	"clinic-api/internal/database"
	"clinic-api/internal/database/store"

	"github.com/labstack/echo/v4"
)

// openLoginTestDB opens a fresh, migrated database (with the seeded
// super-admin) under a throwaway key for one test.
func openLoginTestDB(t *testing.T) {
	t.Helper()
	if err := database.SetKey(strings.Repeat("0", 62) + "a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Open(filepath.Join(t.TempDir(), "login.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
}

// recordComparisons makes comparePassword note the hash of every call, while
// still running the real comparison, until the test ends.
func recordComparisons(t *testing.T) *[]string {
	t.Helper()
	var hashes []string
	compare := comparePassword
	comparePassword = func(password, encodedHash string) (bool, error) {
		hashes = append(hashes, encodedHash)
		return compare(password, encodedHash)
	}
	t.Cleanup(func() { comparePassword = compare })
	return &hashes
}

func postLogin(t *testing.T, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(LoginRequest{Username: username, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := Login(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	return rec
}

// hashCost describes what sets the cost of a comparison against an encoded
// hash: the algorithm, its version and parameters, and the salt and key sizes.
func hashCost(t *testing.T, encoded string) string {
	t.Helper()
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		t.Fatalf("malformed hash: %d fields, want 6", len(parts))
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%s %s %s salt=%d key=%d", parts[1], parts[2], parts[3], len(salt), len(key))
}

// The dummy hash is a real hash with the parameters of a freshly stored one,
// so comparing against it costs what comparing against a stored hash costs.
func TestLoginDummyHash_HasTheCostOfAStoredHash(t *testing.T) {
	fresh, err := auth.HashPassword("a freshly stored password")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hashCost(t, loginDummyHash()), hashCost(t, fresh); got != want {
		t.Errorf("dummy hash: %s, want %s", got, want)
	}
	if _, err := auth.ComparePassword("any password", loginDummyHash()); err != nil {
		t.Errorf("comparing against the dummy hash: %v", err)
	}
	if loginDummyHash() != loginDummyHash() {
		t.Error("the dummy hash is built more than once")
	}
}

// Every refusal answers alike and runs exactly one comparison: a wrong
// password against the stored hash, an unknown username and a disabled
// account against the dummy hash.
func TestLogin_EveryRefusalRunsOneComparison(t *testing.T) {
	openLoginTestDB(t)
	var admin store.User
	if err := admin.GetByUsername("super-admin"); err != nil {
		t.Fatal(err)
	}
	stored, err := auth.HashPassword("right-pw")
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.UpdatePassword(stored); err != nil {
		t.Fatal(err)
	}
	hashes := recordComparisons(t)

	var firstBody string
	for _, tc := range []struct {
		name, username, password, setup, wantHash string
	}{
		{"wrong password", "super-admin", "wrong-pw", "", stored},
		{"unknown username", "nobody-here", "wrong-pw", "", loginDummyHash()},
		{"disabled account, right password", "super-admin", "right-pw",
			`UPDATE users SET is_active = 0 WHERE username = 'super-admin'`, loginDummyHash()},
		{"disabled account without a password yet", "super-admin", "new-pw",
			`UPDATE users SET password_hash = '' WHERE username = 'super-admin'`, loginDummyHash()},
	} {
		if tc.setup != "" {
			if _, err := store.DB.Exec(tc.setup); err != nil {
				t.Fatal(err)
			}
		}
		*hashes = nil
		rec := postLogin(t, tc.username, tc.password)

		var body struct{ Error string }
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusUnauthorized || body.Error != "Invalid username or password" {
			t.Errorf("%s: %d %q, want 401 \"Invalid username or password\"", tc.name, rec.Code, body.Error)
		}
		if firstBody == "" {
			firstBody = rec.Body.String()
		} else if rec.Body.String() != firstBody {
			t.Errorf("%s: body %q differs from the wrong-password body %q", tc.name, rec.Body.String(), firstBody)
		}
		if len(*hashes) != 1 {
			t.Errorf("%s: %d comparisons, want 1", tc.name, len(*hashes))
		} else if (*hashes)[0] != tc.wantHash {
			t.Errorf("%s: compared against the wrong hash", tc.name)
		}
	}

	// The refused sign-in of a disabled account never sets its password.
	var after store.User
	if err := after.GetByUsername("super-admin"); err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash != "" {
		t.Error("a disabled account's first sign-in set its password")
	}
}
