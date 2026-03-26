package utils

import (
	"errors"
	"time"

	"clinic-api/internal/config"
	"clinic-api/internal/database/models"

	"github.com/golang-jwt/jwt/v5"
)

func GenerateToken(user models.User) (string, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(config.JWTLifetime)

	claims := jwt.MapClaims{
		"sub":  user.ID,
		"role": user.Role,
		"exp":  expiresAt.Unix(),
		"iat":  now.Unix(),
		"iss":  "clinic-api",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(config.JWTSecret))
	if err != nil {
		return "", err
	}

	t := models.Token{
		Token:     signed,
		UserID:    user.ID,
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
	}
	if err := t.Create(); err != nil {
		return "", errors.New("failed to store token")
	}

	return signed, nil
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