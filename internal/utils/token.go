package utils

import (
	"errors"
	"time"

	"clinic-api/internal/config"
	"clinic-api/internal/database/models"

	"github.com/golang-jwt/jwt/v5"
)

type TokenResult struct {
	Token     string   `json:"token"`
	ExpiresAt int64    `json:"expiresAt"`
	User      string   `json:"user"`
	Role      string   `json:"role"`
	Scopes    []string `json:"scopes"`
}

func GenerateToken(user models.User, scopes []string) (TokenResult, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(config.JWTLifetime)

	claims := jwt.MapClaims{
		"sub":    user.ID,
		"role":   user.Role,
		"scopes": scopes,
		"exp":    expiresAt.Unix(),
		"iat":    now.Unix(),
		"iss":    "clinic-api",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(config.JWTSecret))
	if err != nil {
		return TokenResult{}, err
	}

	t := models.Token{
		Token:     signed,
		UserID:    user.ID,
		ExpiresAt: models.DateFrom(expiresAt),
	}
	if err := t.Create(); err != nil {
		return TokenResult{}, errors.New("failed to store token")
	}

	return TokenResult{
		Token:     signed,
		ExpiresAt: expiresAt.Unix(),
		User:      user.DisplayName,
		Role:      user.Role,
		Scopes:    scopes,
	}, nil
}

func ParseToken(tokenStr string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(config.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}

	return claims, nil
}