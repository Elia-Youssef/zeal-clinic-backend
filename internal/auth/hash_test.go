package auth

import (
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

func TestHashPassword_Format(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == "" {
		t.Fatal("hash is empty")
	}
	parts := strings.Split(h, "$")
	if len(parts) != 6 {
		t.Fatalf("expected 6 segments, got %d in %q", len(parts), h)
	}
	if parts[0] != "" {
		t.Errorf("expected leading empty segment, got %q", parts[0])
	}
	if parts[1] != "argon2id" {
		t.Errorf("algorithm marker = %q, want argon2id", parts[1])
	}
	if !strings.HasPrefix(parts[2], "v=") {
		t.Errorf("version segment = %q", parts[2])
	}
	if !strings.HasPrefix(parts[3], "m=") || !strings.Contains(parts[3], "t=") || !strings.Contains(parts[3], "p=") {
		t.Errorf("params segment = %q", parts[3])
	}
}

func TestHashPassword_DistinctSalt(t *testing.T) {
	// Same password should produce different hashes due to fresh random salt.
	h1, err := HashPassword("samePass!")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashPassword("samePass!")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Errorf("two hashes of same password must differ (salt randomness)")
	}
}

func TestComparePassword_Match(t *testing.T) {
	pw := "p@ssw0rd!"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ComparePassword(pw, h)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Errorf("expected match")
	}
}

func TestComparePassword_Mismatch(t *testing.T) {
	h, err := HashPassword("right")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ComparePassword("wrong", h)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Errorf("expected mismatch")
	}
}

func TestComparePassword_EmptyPasswordMatchesEmptyHash(t *testing.T) {
	// Hashing the empty string should be a valid round-trip.
	h, err := HashPassword("")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ComparePassword("", h)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("empty password should round-trip")
	}
	// And mismatch with non-empty.
	if ok2, _ := ComparePassword("x", h); ok2 {
		t.Errorf("non-empty must not match empty-password hash")
	}
}

func TestComparePassword_LongPassword(t *testing.T) {
	pw := strings.Repeat("a", 4096)
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ComparePassword(pw, h)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("long password must round-trip")
	}
}

func TestComparePassword_UnicodePassword(t *testing.T) {
	pw := "p∆ssword🔐👨‍👩‍👧"
	h, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := ComparePassword(pw, h)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Errorf("unicode password must round-trip")
	}
}

func TestComparePassword_MalformedHash(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"too few segments": "$argon2id$v=19$x",
		"non-numeric version": "$argon2id$v=abc$m=65536,t=3,p=2$" +
			b64("salt") + "$" + b64("hash"),
		"missing version prefix": "$argon2id$19$m=65536,t=3,p=2$" +
			b64("salt") + "$" + b64("hash"),
		"bad params":        "$argon2id$v=19$broken$" + b64("salt") + "$" + b64("hash"),
		"bad salt b64":      "$argon2id$v=19$m=65536,t=3,p=2$!!!notb64$" + b64("hash"),
		"bad hash b64":      "$argon2id$v=19$m=65536,t=3,p=2$" + b64("salt") + "$!!!",
		"too many segments": "$argon2id$v=19$m=65536,t=3,p=2$x$y$extra",
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ComparePassword("anything", h)
			if err == nil {
				t.Errorf("expected error for %q", h)
			}
		})
	}
}

func TestComparePassword_IncompatibleVersion(t *testing.T) {
	// Force a version that does not match argon2.Version.
	bad := "$argon2id$v=1$m=65536,t=3,p=2$" + b64("salt") + "$" + b64("hash")
	if _, err := ComparePassword("x", bad); err == nil {
		t.Errorf("expected incompatible version error")
	}
}

func TestComparePassword_MatchesArgon2idKDF(t *testing.T) {
	// Sanity-check that ComparePassword uses argon2.IDKey with the embedded
	// params: build a hash by hand and verify it matches the password.
	salt := []byte("0123456789abcdef") // 16 bytes
	memory := uint32(64 * 1024)
	iters := uint32(2)
	par := uint8(1)
	keyLen := uint32(32)
	pw := "manual"
	key := argon2.IDKey([]byte(pw), salt, iters, memory, par, keyLen)
	encoded := "$argon2id$v=" + itoa(argon2.Version) +
		"$m=" + itoa(int(memory)) + ",t=" + itoa(int(iters)) + ",p=" + itoa(int(par)) +
		"$" + b64(string(salt)) + "$" + b64Bytes(key)

	ok, err := ComparePassword(pw, encoded)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Errorf("manual argon2id-encoded hash should match")
	}
	if ok2, _ := ComparePassword("not-it", encoded); ok2 {
		t.Errorf("wrong password should not match manual hash")
	}
}

// The argon2 reference vector for argon2id (password "password", salt
// "somesalt", t=1, m=64, p=1, 24-byte key) in the stored format, so hashes
// written by earlier builds keep verifying after a library upgrade.
func TestComparePassword_KnownAnswerVector(t *testing.T) {
	const vector = "$argon2id$v=19$m=64,t=1,p=1$c29tZXNhbHQ$ZVrRXqxlLcWfcXCnMyv0m4Rpvh/bnCi7"
	if ok, err := ComparePassword("password", vector); err != nil || !ok {
		t.Errorf("reference vector: ok=%v err=%v, want a match", ok, err)
	}
	if ok, _ := ComparePassword("Password", vector); ok {
		t.Error("a near-miss password matched the reference vector")
	}
}

// New hashes use exactly these parameters, a 16-byte salt and a 32-byte key.
func TestHashPassword_PinnedParameters(t *testing.T) {
	h, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(h, "$")
	if len(parts) != 6 || parts[2] != "v=19" || parts[3] != "m=32768,t=5,p=2" {
		t.Fatalf("hash %q, want v=19 and m=32768,t=5,p=2", h)
	}
	salt, errSalt := base64.RawStdEncoding.DecodeString(parts[4])
	key, errKey := base64.RawStdEncoding.DecodeString(parts[5])
	if errSalt != nil || errKey != nil || len(salt) != 16 || len(key) != 32 {
		t.Errorf("salt %d bytes (%v), key %d bytes (%v), want 16 and 32", len(salt), errSalt, len(key), errKey)
	}
}

// helpers
func b64(s string) string      { return b64Bytes([]byte(s)) }
func b64Bytes(b []byte) string { return rawStdNoPad(b) }

// Inline base64.RawStdEncoding to avoid importing in test signatures cleanly.
func rawStdNoPad(b []byte) string {
	const tbl = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out strings.Builder
	n := len(b)
	for i := 0; i < n; i += 3 {
		var c0, c1, c2 byte
		c0 = b[i]
		if i+1 < n {
			c1 = b[i+1]
		}
		if i+2 < n {
			c2 = b[i+2]
		}
		out.WriteByte(tbl[c0>>2])
		out.WriteByte(tbl[((c0&0x03)<<4)|(c1>>4)])
		if i+1 < n {
			out.WriteByte(tbl[((c1&0x0f)<<2)|(c2>>6)])
		}
		if i+2 < n {
			out.WriteByte(tbl[c2&0x3f])
		}
	}
	return out.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	n := len(buf)
	for i > 0 {
		n--
		buf[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}
