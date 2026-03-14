package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
)

// PgSessionStore implements SessionStore using PostgreSQL via sqlc queries.
type PgSessionStore struct {
	queries *db.Queries
}

// NewPgSessionStore creates a session store backed by PostgreSQL.
func NewPgSessionStore(queries *db.Queries) *PgSessionStore {
	return &PgSessionStore{queries: queries}
}

func (s *PgSessionStore) Create(ctx context.Context, params CreateSessionParams) (Session, error) {
	row, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		ID:        params.ID,
		UserID:    params.UserID,
		TokenHash: params.TokenHash,
		ExpiresAt: params.ExpiresAt,
	})
	if err != nil {
		return Session{}, err
	}
	return Session{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *PgSessionStore) FindByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	row, err := s.queries.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, err
	}
	return Session{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		CreatedAt: row.CreatedAt,
	}, nil
}

func (s *PgSessionStore) DeleteByID(ctx context.Context, id uuid.UUID) error {
	return s.queries.DeleteSession(ctx, id)
}

func (s *PgSessionStore) DeleteByUserID(ctx context.Context, userID uuid.UUID) error {
	return s.queries.DeleteSessionsByUserID(ctx, userID)
}

func (s *PgSessionStore) DeleteExpired(ctx context.Context) (int64, error) {
	return s.queries.DeleteExpiredSessions(ctx)
}
