package token

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type (
	Generator struct {
		jwtSecret            []byte
		accessTokenDuration  time.Duration
		refreshTokenDuration time.Duration
	}

	Claims struct {
		jwt.RegisteredClaims

		AccountID   uuid.UUID `json:"account_id"`
		PhoneNumber string    `json:"phone_number"`
		Roles       []string  `json:"roles"`
	}
)

func NewGenerator(
	jwtSecret []byte,
	accessTokenDuration time.Duration,
	refreshTokenDuration time.Duration,
) *Generator {
	return &Generator{
		jwtSecret:            jwtSecret,
		accessTokenDuration:  accessTokenDuration,
		refreshTokenDuration: refreshTokenDuration,
	}
}

func (g *Generator) GenerateAccessToken(accountID uuid.UUID) (string, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(g.accessTokenDuration)

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   accountID.String(),
			Issuer:    "rekreativko",
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(g.jwtSecret)
}

func (g *Generator) ValidateAccessToken(token string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		return g.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !t.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := t.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

func (g *Generator) GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)

	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to read random: %w", err)
	}

	return base64.URLEncoding.EncodeToString(b), nil
}

func (g *Generator) HashRefreshToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash refresh token: %w", err)
	}

	return string(hash), nil
}

func (g *Generator) CompareRefreshTokenAndHash(token, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token))
}

func (g *Generator) RefreshTokenDuration() time.Duration {
	return g.refreshTokenDuration
}

func (g *Generator) AccessTokenDuration() time.Duration {
	return g.accessTokenDuration
}
