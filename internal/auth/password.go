package auth

import (
	"errors"
	"strings"
)

// A password is used exactly as its owner types it: no sign-in and no
// set-password path trims it. A password of spaces only is refused, so an
// account never holds one; on a set path an empty password means no password
// was given, and the account takes its password from a first sign-in.

// ErrBlankPassword refuses a password of spaces only.
var ErrBlankPassword = errors.New("password is blank")

// BlankPassword reports whether a typed password is empty or spaces only.
func BlankPassword(password string) bool {
	return strings.TrimSpace(password) == ""
}

// HashNewPassword hashes a password for setting on an account, exactly as
// typed. An empty password means none was given and hashes to none; a
// password of spaces only is refused.
func HashNewPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	if BlankPassword(password) {
		return "", ErrBlankPassword
	}
	return HashPassword(password)
}
