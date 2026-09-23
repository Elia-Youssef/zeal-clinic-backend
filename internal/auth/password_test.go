package auth

import (
	"errors"
	"testing"
)

// A password is hashed exactly as typed, an empty one means none was given,
// and one of spaces only is refused.
func TestHashNewPassword_AsTypedBlankRefused(t *testing.T) {
	hash, err := HashNewPassword("  round trip  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok, err := ComparePassword("  round trip  ", hash); err != nil || !ok {
		t.Errorf("the password as typed: ok=%v err=%v, want a match", ok, err)
	}
	if ok, _ := ComparePassword("round trip", hash); ok {
		t.Error("the trimmed password matched")
	}

	blank, err := HashNewPassword("   ")
	if !errors.Is(err, ErrBlankPassword) {
		t.Fatalf("spaces-only password: err=%v, want ErrBlankPassword", err)
	}
	if blank != "" {
		t.Errorf("spaces-only password: hash %q, want none", blank)
	}

	none, err := HashNewPassword("")
	if err != nil {
		t.Fatalf("empty password: %v", err)
	}
	if none != "" {
		t.Errorf("empty password: hash %q, want none", none)
	}
}

func TestBlankPassword(t *testing.T) {
	for _, pw := range []string{"", " ", "\t\n "} {
		if !BlankPassword(pw) {
			t.Errorf("BlankPassword(%q) = false, want true", pw)
		}
	}
	for _, pw := range []string{"x", " x", "x "} {
		if BlankPassword(pw) {
			t.Errorf("BlankPassword(%q) = true, want false", pw)
		}
	}
}
