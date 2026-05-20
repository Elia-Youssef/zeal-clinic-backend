package auth

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"clinic-api/internal/config"
	"clinic-api/internal/database/migrations"
	"clinic-api/internal/database/store"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/vfs/adiantum"
	"github.com/pressly/goose/v3"
)

const testHexKey = "00000000000000000000000000000000000000000000000000000000000000ff"

var (
	tokenDBCounter atomic.Uint64
	tokenGooseOnce sync.Once
	tokenCfgOnce   sync.Once
)

func setupAuthTestEnv(t *testing.T) {
	t.Helper()

	tokenGooseOnce.Do(func() {
		goose.SetBaseFS(migrations.FS)
		goose.SetLogger(goose.NopLogger())
		if err := goose.SetDialect("sqlite3"); err != nil {
			t.Fatalf("goose dialect: %v", err)
		}
	})
	tokenCfgOnce.Do(func() {
		if os.Getenv("DB_ENCRYPTION_KEY") == "" {
			os.Setenv("DB_ENCRYPTION_KEY", testHexKey)
		}
		_ = config.Load()
	})

	id := tokenDBCounter.Add(1)
	dbPath := filepath.Join(t.TempDir(), fmt.Sprintf("auth-test-%d.db", id))
	dsn := "file:" + filepath.ToSlash(dbPath) +
		"?vfs=adiantum&hexkey=" + testHexKey +
		"&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"

	w, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open write: %v", err)
	}
	w.SetMaxOpenConns(1)
	if err := goose.Up(w, "."); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open read: %v", err)
	}
	r.SetMaxOpenConns(4)
	prevDB, prevRDB := store.DB, store.RDB
	store.DB, store.RDB = w, r
	t.Cleanup(func() {
		w.Close()
		r.Close()
		store.DB, store.RDB = prevDB, prevRDB
	})
}

func makeTokenUser(t *testing.T, username string) store.User {
	t.Helper()
	u := store.User{
		Username:    username,
		DisplayName: "Test " + username,
		Role:        "admin",
		IsActive:    true,
	}
	if err := u.Create("hash"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

func TestGenerateToken_PersistsTokenRow(t *testing.T) {
	setupAuthTestEnv(t)
	u := makeTokenUser(t, "alice")

	tr, err := GenerateToken(u, []string{"patients:read"})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if tr.Token == "" {
		t.Fatal("empty token")
	}
	if tr.User != u.DisplayName {
		t.Errorf("User = %q want %q", tr.User, u.DisplayName)
	}
	if tr.Role != "admin" {
		t.Errorf("Role = %q want admin", tr.Role)
	}
	if len(tr.Scopes) != 1 || tr.Scopes[0] != "patients:read" {
		t.Errorf("Scopes = %v", tr.Scopes)
	}

	var got store.Token
	if err := got.GetByValue(tr.Token); err != nil {
		t.Fatalf("token not persisted: %v", err)
	}
	if got.UserID != u.ID {
		t.Errorf("UserID = %q want %q", got.UserID, u.ID)
	}
}

func TestGenerateToken_RapidRepeatProducesDistinctTokens(t *testing.T) {
	// Two logins in the same second get distinct JWTs through the jti claim,
	// so they don't collide on the tokens.token UNIQUE constraint.
	setupAuthTestEnv(t)
	u := makeTokenUser(t, "bob")

	tr1, err := GenerateToken(u, []string{"patients:read"})
	if err != nil {
		t.Fatalf("GenerateToken 1: %v", err)
	}
	tr2, err := GenerateToken(u, []string{"patients:read"})
	if err != nil {
		t.Fatalf("GenerateToken 2: %v", err)
	}
	if tr1.Token == tr2.Token {
		t.Errorf("rapid repeat must yield distinct tokens, got the same JWT")
	}
}

func TestGenerateToken_ClaimsShape(t *testing.T) {
	setupAuthTestEnv(t)
	u := makeTokenUser(t, "carol")

	tr, err := GenerateToken(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(tr.Token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims["sub"] != u.ID {
		t.Errorf("sub = %v want %q", claims["sub"], u.ID)
	}
	if claims["role"] != "admin" {
		t.Errorf("role = %v want admin", claims["role"])
	}
	if claims["iss"] != "clinic-api" {
		t.Errorf("iss = %v want clinic-api", claims["iss"])
	}
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		t.Errorf("jti missing or wrong type: %v", claims["jti"])
	}
	// exp must be in the future, iat must be at or before now.
	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatalf("exp not numeric: %v", claims["exp"])
	}
	if int64(exp) <= time.Now().Unix() {
		t.Errorf("exp not in future: %v", exp)
	}
}

func TestParseToken_ValidToken(t *testing.T) {
	setupAuthTestEnv(t)
	u := makeTokenUser(t, "dave")

	tr, err := GenerateToken(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(tr.Token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims["sub"] != u.ID {
		t.Errorf("sub = %v want %q", claims["sub"], u.ID)
	}
}

func TestParseToken_Garbage(t *testing.T) {
	setupAuthTestEnv(t)
	for _, in := range []string{"", "not.a.jwt", "x.y.z", "definitely-bad"} {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseToken(in); err == nil {
				t.Errorf("expected error for %q", in)
			}
		})
	}
}

func TestParseToken_TamperedSignature(t *testing.T) {
	setupAuthTestEnv(t)
	u := makeTokenUser(t, "eve")
	tr, err := GenerateToken(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	tampered := tr.Token[:len(tr.Token)-4] + "AAAA"
	if _, err := ParseToken(tampered); err == nil {
		t.Errorf("expected error on tampered signature")
	}
}

func TestParseToken_WrongSigningAlgorithmRejected(t *testing.T) {
	setupAuthTestEnv(t)

	// Build a token signed with the "none" alg (no signature). Must be rejected.
	claims := jwt.MapClaims{"sub": "x", "role": "admin", "iss": "clinic-api"}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(signed); err == nil {
		t.Errorf("ParseToken accepted alg=none")
	}
}

func TestParseToken_ExpiredRejected(t *testing.T) {
	setupAuthTestEnv(t)

	cfg := config.Current()
	claims := jwt.MapClaims{
		"sub":  "u",
		"role": "admin",
		"iss":  "clinic-api",
		"iat":  time.Now().Add(-2 * time.Hour).Unix(),
		"exp":  time.Now().Add(-time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(signed); err == nil {
		t.Errorf("expected error for expired token")
	}
}

func TestParseToken_WrongSecretRejected(t *testing.T) {
	setupAuthTestEnv(t)

	claims := jwt.MapClaims{
		"sub":  "u",
		"role": "admin",
		"iss":  "clinic-api",
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte("not-the-right-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(signed); err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("expected invalid token error, got %v", err)
	}
}

func TestGenerateToken_RespectsJWTLifetime(t *testing.T) {
	setupAuthTestEnv(t)
	u := makeTokenUser(t, "frank")

	tr, err := GenerateToken(u, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Current()
	now := time.Now().Unix()
	delta := tr.ExpiresAt - now
	tolerance := int64(5)
	want := int64(cfg.JWTLifetime / time.Second)
	if delta < want-tolerance || delta > want+tolerance {
		t.Errorf("ExpiresAt delta = %ds, want ~%ds", delta, want)
	}
}
