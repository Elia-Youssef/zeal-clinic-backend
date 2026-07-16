package auth

import (
	"errors"
	"time"

	"clinic-api/internal/config"
	"clinic-api/internal/database/store"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenResult struct {
	Token      string   `json:"token"`
	ExpiresAt  int64    `json:"expiresAt"`
	UserID     string   `json:"userId"`
	User       string   `json:"user"`
	Role       string   `json:"role"`
	Scopes     []string `json:"scopes"`
	EmployeeID *string  `json:"employeeId,omitempty"`
}

func GenerateToken(user store.User, grantedScopes []string) (TokenResult, error) {
	cfg := config.Current()
	now := time.Now().UTC()
	expiresAt := now.Add(cfg.JWTLifetime)

	claims := jwt.MapClaims{
		"sub":  user.ID,
		"role": user.Role,
		"exp":  expiresAt.Unix(),
		"iat":  now.Unix(),
		"iss":  "clinic-api",
		// jti makes each token unique even when iat/exp resolve to the same
		// second (otherwise rapid repeat logins collide on tokens.token UNIQUE).
		"jti": uuid.Must(uuid.NewV7()).String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		return TokenResult{}, err
	}

	t := store.Token{
		Token:     signed,
		UserID:    user.ID,
		ExpiresAt: store.DateFrom(expiresAt),
	}
	if err := t.Create(); err != nil {
		return TokenResult{}, errors.New("failed to store token")
	}

	result := TokenResult{
		Token:     signed,
		ExpiresAt: expiresAt.Unix(),
		UserID:    user.ID,
		User:      user.DisplayName,
		Role:      user.Role,
		Scopes:    grantedScopes,
	}
	if empID, err := store.EmployeeIDForUser(user.ID); err == nil {
		result.EmployeeID = &empID
	}
	return result, nil
}

func ParseToken(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.Current().JWTSecret), nil
	}, jwt.WithExpirationRequired(), jwt.WithIssuer("clinic-api"))
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	return claims, nil
}
