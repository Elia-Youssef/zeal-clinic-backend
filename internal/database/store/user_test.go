package store

import (
	"errors"
	"strings"
	"testing"

	"clinic-api/internal/validation"
)

// validUser returns a User struct that passes IsValid (used as a base for
// negative assertions where we mutate one field).
func validUser() User {
	return User{
		Username:    "alice",
		DisplayName: "Alice",
		Role:        "staff",
		IsActive:    true,
	}
}

func TestUser_IsValid_Ok(t *testing.T) {
	u := validUser()
	if err := u.IsValid(); err != nil {
		t.Errorf("expected ok, got %v", err)
	}
}

func TestUser_IsValid_Errors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*User)
		key  string
	}{
		{"missing username", func(u *User) { u.Username = "" }, "username"},
		{"username too short", func(u *User) { u.Username = "ab" }, "username"},
		{"missing display name", func(u *User) { u.DisplayName = "" }, "displayName"},
		{"missing role", func(u *User) { u.Role = "" }, "role"},
		{"unknown role", func(u *User) { u.Role = "intruder" }, "role"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := validUser()
			tc.mut(&u)
			err := u.IsValid()
			if err == nil {
				t.Fatalf("expected error")
			}
			var ve validation.Errors
			if !errors.As(err, &ve) {
				t.Fatalf("err is %T, want validation.Errors", err)
			}
			if _, ok := ve[tc.key]; !ok {
				t.Errorf("expected key %q in %v", tc.key, ve)
			}
		})
	}
}

func TestUser_IsValid_AcceptsAllValidRoles(t *testing.T) {
	for _, role := range []string{"super-admin", "admin", "staff", "nurse"} {
		u := validUser()
		u.Role = role
		if err := u.IsValid(); err != nil {
			t.Errorf("role %q: unexpected err %v", role, err)
		}
	}
}

func TestUser_Create_AssignsIDAndTimestamps(t *testing.T) {
	setupTestDB(t)
	u := User{Username: "bob", DisplayName: "Bob", Role: "staff", IsActive: true}
	if err := u.Create("hash"); err != nil {
		t.Fatal(err)
	}
	if u.ID == "" {
		t.Errorf("ID not assigned")
	}
	if u.CreatedAt == "" || u.UpdatedAt == "" {
		t.Errorf("timestamps not set: created=%q updated=%q", u.CreatedAt, u.UpdatedAt)
	}
	if u.PasswordHash != "hash" {
		t.Errorf("PasswordHash = %q", u.PasswordHash)
	}

	// Round-trip via GetByID.
	var got User
	if err := got.GetByID(u.ID); err != nil {
		t.Fatal(err)
	}
	if got.Username != "bob" || !got.IsActive {
		t.Errorf("got %+v", got)
	}
}

func TestUser_Create_DuplicateUsernameRejected(t *testing.T) {
	setupTestDB(t)
	u1 := User{Username: "dup", DisplayName: "X", Role: "staff", IsActive: true}
	u2 := User{Username: "dup", DisplayName: "Y", Role: "staff", IsActive: true}
	if err := u1.Create("h1"); err != nil {
		t.Fatal(err)
	}
	err := u2.Create("h2")
	if err == nil {
		t.Errorf("expected unique constraint violation")
	}
}

func TestUser_GetByID_NotFound(t *testing.T) {
	setupTestDB(t)
	var u User
	err := u.GetByID("does-not-exist")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUser_GetByUsername_RoundTrip(t *testing.T) {
	setupTestDB(t)
	u := User{Username: "carol", DisplayName: "C", Role: "admin", IsActive: true}
	if err := u.Create("h"); err != nil {
		t.Fatal(err)
	}
	var got User
	if err := got.GetByUsername("carol"); err != nil {
		t.Fatal(err)
	}
	if got.ID != u.ID {
		t.Errorf("got %v want %v", got.ID, u.ID)
	}
}

func TestUser_GetByUsername_NotFound(t *testing.T) {
	setupTestDB(t)
	var u User
	err := u.GetByUsername("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestUser_Update_PartialFieldsOnly(t *testing.T) {
	setupTestDB(t)
	u := User{Username: "old", DisplayName: "Old", Role: "staff", IsActive: true}
	if err := u.Create("h"); err != nil {
		t.Fatal(err)
	}
	prevHash := u.PasswordHash
	// Force an obviously-old UpdatedAt in the row so we can detect that Update
	// rewrites it. Using the value field directly avoids races against
	// DateNow's 1-second RFC3339 resolution (two writes in the same second
	// produce identical timestamps).
	if _, err := DB.Exec(`UPDATE users SET updated_at = ? WHERE id = ?`, "2000-01-01T00:00:00Z", u.ID); err != nil {
		t.Fatal(err)
	}

	if err := u.Update(map[string]any{
		"displayName": "New",
		"role":        "admin",
	}); err != nil {
		t.Fatal(err)
	}
	if u.DisplayName != "New" || u.Role != "admin" {
		t.Errorf("update did not apply, got %+v", u)
	}
	if u.Username != "old" {
		t.Errorf("username should be untouched, got %q", u.Username)
	}
	if u.PasswordHash != prevHash {
		t.Errorf("password hash should not change on Update, got %q", u.PasswordHash)
	}
	if u.UpdatedAt == "2000-01-01T00:00:00Z" {
		t.Errorf("UpdatedAt should be rewritten on Update")
	}
}

func TestUser_Update_BoolIsActive(t *testing.T) {
	setupTestDB(t)
	u := User{Username: "act", DisplayName: "A", Role: "staff", IsActive: true}
	if err := u.Create("h"); err != nil {
		t.Fatal(err)
	}
	if err := u.Update(map[string]any{"isActive": false}); err != nil {
		t.Fatal(err)
	}
	if u.IsActive {
		t.Errorf("IsActive should be false")
	}

	// Re-enable.
	if err := u.Update(map[string]any{"isActive": true}); err != nil {
		t.Fatal(err)
	}
	if !u.IsActive {
		t.Errorf("IsActive should be true again")
	}
}

func TestUser_Update_EmptyMapJustReloads(t *testing.T) {
	setupTestDB(t)
	u := User{Username: "noop", DisplayName: "N", Role: "staff", IsActive: true}
	if err := u.Create("h"); err != nil {
		t.Fatal(err)
	}
	prev := u.UpdatedAt
	// Tamper in-memory and verify Update with no recognized keys reloads from DB.
	u.DisplayName = "tampered"
	if err := u.Update(map[string]any{"unknownKey": "x"}); err != nil {
		t.Fatal(err)
	}
	if u.DisplayName != "N" {
		t.Errorf("expected DB reload to reset DisplayName, got %q", u.DisplayName)
	}
	if u.UpdatedAt != prev {
		t.Errorf("empty update should NOT touch UpdatedAt, got %v want %v", u.UpdatedAt, prev)
	}
}

func TestUser_UpdatePassword(t *testing.T) {
	setupTestDB(t)
	u := User{Username: "pw", DisplayName: "P", Role: "staff", IsActive: true}
	if err := u.Create("oldhash"); err != nil {
		t.Fatal(err)
	}
	if err := u.UpdatePassword("newhash"); err != nil {
		t.Fatal(err)
	}
	// Reload to verify.
	var got User
	if err := got.GetByID(u.ID); err != nil {
		t.Fatal(err)
	}
	if got.PasswordHash != "newhash" {
		t.Errorf("PasswordHash = %q", got.PasswordHash)
	}
}

func TestUserList_GetAll_PaginationAndFilter(t *testing.T) {
	setupTestDB(t)
	// Account for seeded users visible to the management list.
	var preTotal int
	RDB.QueryRow("SELECT COUNT(*) FROM users WHERE role != 'super-admin'").Scan(&preTotal)

	for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo"} {
		u := User{Username: name, DisplayName: name + " full", Role: "staff", IsActive: true}
		if err := u.Create("h"); err != nil {
			t.Fatal(err)
		}
	}

	// All
	var list UserList
	total, err := list.GetAll(ListParams{})
	if err != nil {
		t.Fatal(err)
	}
	if total != preTotal+5 {
		t.Errorf("total = %d want %d", total, preTotal+5)
	}

	// Filter by username substring.
	list = nil
	total, err = list.GetAll(ListParams{Filter: "alph"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("filter total = %d want 1", total)
	}
	if len(list) != 1 || !strings.Contains(list[0].Username, "alph") {
		t.Errorf("got %+v", list)
	}

	// Pagination
	list = nil
	_, err = list.GetAll(ListParams{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("limit=2 returned %d rows", len(list))
	}
}

// Token tests

// userForTokens creates a user (FK target) and returns it.
func userForTokens(t *testing.T) User {
	t.Helper()
	u := User{Username: "tok-user", DisplayName: "T", Role: "staff", IsActive: true}
	if err := u.Create("h"); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestToken_Create_RoundTrip(t *testing.T) {
	setupTestDB(t)
	u := userForTokens(t)
	tok := Token{Token: "abc123", UserID: u.ID, ExpiresAt: DateNow()}
	if err := tok.Create(); err != nil {
		t.Fatal(err)
	}
	// Lookup by value.
	var got Token
	if err := got.GetByValue("abc123"); err != nil {
		t.Fatal(err)
	}
	if got.UserID != u.ID {
		t.Errorf("UserID = %q want %q", got.UserID, u.ID)
	}
	if got.ID == "" || got.CreatedAt == "" {
		t.Errorf("id/createdAt should be assigned, got %+v", got)
	}
}

func TestToken_GetByValue_NotFound(t *testing.T) {
	setupTestDB(t)
	var tok Token
	err := tok.GetByValue("nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestToken_Delete(t *testing.T) {
	setupTestDB(t)
	u := userForTokens(t)
	tok := Token{Token: "del-me", UserID: u.ID, ExpiresAt: DateNow()}
	if err := tok.Create(); err != nil {
		t.Fatal(err)
	}
	if err := tok.Delete(); err != nil {
		t.Fatal(err)
	}
	var got Token
	if err := got.GetByValue("del-me"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected not found, got %v", err)
	}
}

func TestToken_Delete_Idempotent(t *testing.T) {
	setupTestDB(t)
	tok := Token{Token: "ghost"}
	// Delete on something that never existed should not error (no row affected).
	if err := tok.Delete(); err != nil {
		t.Errorf("deleting non-existent token should not error, got %v", err)
	}
}

func TestToken_DeleteExpired(t *testing.T) {
	setupTestDB(t)
	u := userForTokens(t)
	// Insert one expired and one fresh token.
	expired := Token{Token: "expired", UserID: u.ID, ExpiresAt: Date("2000-01-01T00:00:00Z")}
	if err := expired.Create(); err != nil {
		t.Fatal(err)
	}
	fresh := Token{Token: "fresh", UserID: u.ID, ExpiresAt: Date("2999-01-01T00:00:00Z")}
	if err := fresh.Create(); err != nil {
		t.Fatal(err)
	}

	if err := (&Token{}).DeleteExpired(); err != nil {
		t.Fatal(err)
	}
	var got Token
	if err := got.GetByValue("expired"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired token should be gone, got %v", err)
	}
	if err := got.GetByValue("fresh"); err != nil {
		t.Errorf("fresh token should still exist, got %v", err)
	}
}

func TestToken_Create_ForeignKeyOnUserID(t *testing.T) {
	setupTestDB(t)
	tok := Token{Token: "no-user", UserID: "nope", ExpiresAt: DateNow()}
	err := tok.Create()
	if err == nil {
		t.Errorf("expected FK violation for unknown user")
	}
}

func TestToken_CascadeOnUserDelete(t *testing.T) {
	setupTestDB(t)
	u := userForTokens(t)
	tok := Token{Token: "cascade", UserID: u.ID, ExpiresAt: DateNow()}
	if err := tok.Create(); err != nil {
		t.Fatal(err)
	}
	// Delete the user; the token should cascade away.
	if _, err := DB.Exec(`DELETE FROM users WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	var got Token
	if err := got.GetByValue("cascade"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected cascade delete, got %v", err)
	}
}
