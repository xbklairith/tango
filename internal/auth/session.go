package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

var ErrSessionNotFound = errors.New("auth: session not found")

// CreateSessionParams holds the data needed to create a session.
type CreateSessionParams struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
}

// Session represents a persisted session record.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// SessionStore defines the interface for session persistence.
type SessionStore interface {
	Create(ctx context.Context, params CreateSessionParams) (Session, error)
	FindByTokenHash(ctx context.Context, tokenHash string) (Session, error)
	DeleteByID(ctx context.Context, id uuid.UUID) error
	DeleteByUserID(ctx context.Context, userID uuid.UUID) error
	DeleteExpired(ctx context.Context) (int64, error)
}

// HashToken returns the hex-encoded SHA-256 hash of a raw JWT string.
func HashToken(rawToken string) string {
	h := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(h[:])
}

// StartSessionCleanup launches a goroutine that periodically deletes expired sessions.
func StartSessionCleanup(ctx context.Context, store SessionStore, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := store.DeleteExpired(ctx)
			if err != nil {
				slog.Error("session cleanup failed", "error", err)
				continue
			}
			if deleted > 0 {
				slog.Info("cleaned up expired sessions", "count", deleted)
			}
		}
	}
}
