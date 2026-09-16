package token

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type (
	Generator struct {
		jwtSecret            []byte
		accessTokenDuration  time.Duration
		refreshTokenDuration time.Duration
	}

	// Claims is deliberately minimal - just the standard registered claims
	// (sub carries the account ID). Anything else about the account (email,
	// phone, subscription tier, etc.) is mutable and can't be safely cached in
	// a token that can't be revoked or edited once issued; fetch it fresh via
	// GET /api/v1/me instead.
	Claims struct {
		jwt.RegisteredClaims
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

func (g *Generator) ValidateAccessToken(ctx context.Context, token string) (*Claims, error) {
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

// HashRefreshToken returns a deterministic SHA-256 hex digest of a refresh
// token, for storage/lookup. Unlike passwords, refresh tokens are already
// high-entropy random values (see GenerateRefreshToken), so a slow, salted
// hash like bcrypt isn't needed to resist brute-forcing - a fast, deterministic
// hash is what lets the database look a token up by equality (`WHERE token_hash = ?`)
// while still never storing the raw, usable credential.
func (g *Generator) HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (g *Generator) RefreshTokenDuration() time.Duration {
	return g.refreshTokenDuration
}

func (g *Generator) AccessTokenDuration() time.Duration {
	return g.accessTokenDuration
}
