package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrTokenMalformed = errors.New("auth: token is malformed")
	ErrTokenExpired   = errors.New("auth: token has expired")
	ErrTokenInvalid   = errors.New("auth: token is invalid")
)

// SessionClaims holds the JWT claims for a user session.
type SessionClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
}

// JWTService handles token creation and validation.
type JWTService struct {
	signingKey []byte
	ttl        time.Duration
}

// NewJWTService creates a JWTService with the given signing key and TTL.
// Returns an error if the key is shorter than 32 bytes.
func NewJWTService(signingKey []byte, ttl time.Duration) (*JWTService, error) {
	if len(signingKey) < 32 {
		return nil, fmt.Errorf("auth: signing key must be at least 32 bytes (256 bits), got %d bytes", len(signingKey))
	}
	return &JWTService{
		signingKey: signingKey,
		ttl:        ttl,
	}, nil
}

// Mint creates a new signed JWT for the given user.
func (s *JWTService) Mint(userID uuid.UUID, email string) (string, error) {
	now := time.Now()
	claims := SessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
		},
		Email: email,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.signingKey)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

// Validate parses and validates a JWT string.
func (s *JWTService) Validate(tokenString string) (*SessionClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &SessionClaims{}, func(token *jwt.Token) (any, error) {
		// Reject algorithm-switching attacks
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.signingKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		return nil, ErrTokenInvalid
	}

	claims, ok := token.Claims.(*SessionClaims)
	if !ok || !token.Valid {
		return nil, ErrTokenInvalid
	}

	return claims, nil
}
