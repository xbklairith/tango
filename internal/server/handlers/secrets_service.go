package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/secrets"
)

// SecretsService encapsulates all secret business logic: encryption, CRUD, and injection.
type SecretsService struct {
	queries *db.Queries
	dbConn  *sql.DB
	keyMgr  *secrets.MasterKeyManager
}

// NewSecretsService creates a new SecretsService.
func NewSecretsService(q *db.Queries, dbConn *sql.DB, keyMgr *secrets.MasterKeyManager) *SecretsService {
	return &SecretsService{
		queries: q,
		dbConn:  dbConn,
		keyMgr:  keyMgr,
	}
}

// SecretResponse is the masked representation of a secret for API responses.
type SecretResponse struct {
	ID            uuid.UUID  `json:"id"`
	SquadID       uuid.UUID  `json:"squadId"`
	Name          string     `json:"name"`
	MaskedValue   string     `json:"maskedValue"`
	CreatedAt     string     `json:"createdAt"`
	UpdatedAt     string     `json:"updatedAt"`
	LastRotatedAt *string    `json:"lastRotatedAt,omitempty"`
}

// Create validates, encrypts, and persists a new secret.
func (s *SecretsService) Create(ctx context.Context, squadID uuid.UUID, name, value string) (*SecretResponse, error) {
	if err := domain.ValidateCreateSecretInput(name, value); err != nil {
		return nil, &ServiceError{Code: 400, ErrorCode: "VALIDATION_ERROR", Message: err.Error()}
	}

	// Encrypt
	ciphertext, nonce, err := s.keyMgr.Encrypt([]byte(value))
	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	maskedHint := domain.MaskValue(value)

	// Insert with activity log in a transaction
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	secret, err := qtx.CreateSquadSecret(ctx, db.CreateSquadSecretParams{
		SquadID:        squadID,
		Name:           name,
		EncryptedValue: ciphertext,
		Nonce:          nonce,
		MaskedHint:     maskedHint,
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, &ServiceError{Code: 409, ErrorCode: "SECRET_NAME_CONFLICT", Message: fmt.Sprintf("secret %q already exists in this squad", name)}
		}
		return nil, fmt.Errorf("creating secret: %w", err)
	}

	// Activity log
	actorType, actorID := resolveActor(ctx)
	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "secret.created",
		EntityType: "secret",
		EntityID:   secret.ID,
		Metadata:   map[string]any{"name": name, "squad_id": secret.SquadID.String()},
	}); err != nil {
		return nil, fmt.Errorf("logging activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return secretToResponse(secret), nil
}

// List returns all secrets for a squad with masked values (no decryption).
func (s *SecretsService) List(ctx context.Context, squadID uuid.UUID) ([]SecretResponse, error) {
	rows, err := s.queries.ListSquadSecrets(ctx, squadID)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}

	result := make([]SecretResponse, 0, len(rows))
	for _, row := range rows {
		resp := SecretResponse{
			ID:        row.ID,
			SquadID:   row.SquadID,
			Name:      row.Name,
			MaskedValue: row.MaskedHint,
			CreatedAt: row.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: row.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if row.LastRotatedAt.Valid {
			ts := row.LastRotatedAt.Time.Format("2006-01-02T15:04:05Z")
			resp.LastRotatedAt = &ts
		}
		result = append(result, resp)
	}
	return result, nil
}

// Update re-encrypts a secret with a new value and nonce.
func (s *SecretsService) Update(ctx context.Context, squadID uuid.UUID, name, newValue string) (*SecretResponse, error) {
	if err := domain.ValidateSecretValue(newValue); err != nil {
		return nil, &ServiceError{Code: 400, ErrorCode: "VALIDATION_ERROR", Message: err.Error()}
	}

	// Verify the secret exists
	_, err := s.queries.GetSquadSecretByName(ctx, db.GetSquadSecretByNameParams{
		SquadID: squadID,
		Name:    name,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, &ServiceError{Code: 404, ErrorCode: "SECRET_NOT_FOUND", Message: fmt.Sprintf("secret %q not found", name)}
		}
		return nil, fmt.Errorf("getting secret: %w", err)
	}

	// Encrypt new value
	ciphertext, nonce, err := s.keyMgr.Encrypt([]byte(newValue))
	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	maskedHint := domain.MaskValue(newValue)

	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	updated, err := qtx.UpdateSquadSecretValue(ctx, db.UpdateSquadSecretValueParams{
		EncryptedValue: ciphertext,
		Nonce:          nonce,
		MaskedHint:     maskedHint,
		SquadID:        squadID,
		Name:           name,
	})
	if err != nil {
		return nil, fmt.Errorf("updating secret: %w", err)
	}

	actorType, actorID := resolveActor(ctx)
	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "secret.updated",
		EntityType: "secret",
		EntityID:   updated.ID,
		Metadata:   map[string]any{"name": name, "squad_id": updated.SquadID.String()},
	}); err != nil {
		return nil, fmt.Errorf("logging activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return secretToResponse(updated), nil
}

// Delete removes a secret by squad and name.
func (s *SecretsService) Delete(ctx context.Context, squadID uuid.UUID, name string) error {
	// Verify exists first to get entity ID for activity log
	existing, err := s.queries.GetSquadSecretByName(ctx, db.GetSquadSecretByNameParams{
		SquadID: squadID,
		Name:    name,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return &ServiceError{Code: 404, ErrorCode: "SECRET_NOT_FOUND", Message: fmt.Sprintf("secret %q not found", name)}
		}
		return fmt.Errorf("getting secret: %w", err)
	}

	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	if err := qtx.DeleteSquadSecret(ctx, db.DeleteSquadSecretParams{
		SquadID: squadID,
		Name:    name,
	}); err != nil {
		return fmt.Errorf("deleting secret: %w", err)
	}

	actorType, actorID := resolveActor(ctx)
	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "secret.deleted",
		EntityType: "secret",
		EntityID:   existing.ID,
		Metadata:   map[string]any{"name": name, "squad_id": squadID.String()},
	}); err != nil {
		return fmt.Errorf("logging activity: %w", err)
	}

	return tx.Commit()
}

// GetDecryptedSecrets decrypts all secrets for a squad and returns them as a name-value map.
// Failed decryptions are logged and skipped (not fatal).
func (s *SecretsService) GetDecryptedSecrets(ctx context.Context, squadID uuid.UUID) (map[string]string, error) {
	allSecrets, err := s.queries.ListAllSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing secrets for decryption: %w", err)
	}

	result := make(map[string]string)
	for _, sec := range allSecrets {
		if sec.SquadID != squadID {
			continue
		}
		plaintext, err := s.keyMgr.Decrypt(sec.EncryptedValue, sec.Nonce)
		if err != nil {
			slog.Warn("failed to decrypt secret, skipping",
				"secret_id", sec.ID,
				"secret_name", sec.Name,
				"squad_id", squadID,
				"error", err)
			continue
		}
		result[sec.Name] = string(plaintext)
	}
	return result, nil
}

// RotateMasterKey re-encrypts all secrets with a new master key atomically.
// Returns the count of rotated secrets.
func (s *SecretsService) RotateMasterKey(ctx context.Context) (int, error) {
	// 1. Rotate in-memory key, get old key
	oldKey, err := s.keyMgr.RotateKey()
	if err != nil {
		return 0, fmt.Errorf("rotating key: %w", err)
	}

	// 2. List all secrets
	allSecrets, err := s.queries.ListAllSecrets(ctx)
	if err != nil {
		s.keyMgr.RestoreKey(oldKey)
		return 0, fmt.Errorf("listing secrets for rotation: %w", err)
	}

	if len(allSecrets) == 0 {
		// Persist the new key even with no secrets
		if err := s.keyMgr.PersistKey(); err != nil {
			slog.Error("failed to persist rotated key", "error", err)
		}
		return 0, nil
	}

	// 3. Begin transaction for re-encryption
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		s.keyMgr.RestoreKey(oldKey)
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	for _, sec := range allSecrets {
		// Decrypt with old key
		plaintext, err := secrets.DecryptWithKey(oldKey, sec.EncryptedValue, sec.Nonce)
		if err != nil {
			s.keyMgr.RestoreKey(oldKey)
			return 0, fmt.Errorf("decrypting secret %s during rotation: %w", sec.ID, err)
		}

		// Re-encrypt with new key
		newCiphertext, newNonce, err := s.keyMgr.Encrypt(plaintext)
		if err != nil {
			s.keyMgr.RestoreKey(oldKey)
			return 0, fmt.Errorf("re-encrypting secret %s during rotation: %w", sec.ID, err)
		}

		if err := qtx.UpdateSquadSecretEncryption(ctx, db.UpdateSquadSecretEncryptionParams{
			EncryptedValue: newCiphertext,
			Nonce:          newNonce,
			ID:             sec.ID,
		}); err != nil {
			s.keyMgr.RestoreKey(oldKey)
			return 0, fmt.Errorf("updating secret %s during rotation: %w", sec.ID, err)
		}
	}

	// 4. Commit
	if err := tx.Commit(); err != nil {
		s.keyMgr.RestoreKey(oldKey)
		return 0, fmt.Errorf("committing rotation: %w", err)
	}

	// 5. Persist new key to disk
	if err := s.keyMgr.PersistKey(); err != nil {
		slog.Error("failed to persist rotated key to disk (key is active in memory)", "error", err)
	}

	// 6. Activity log (best-effort, outside rotation tx)
	actorType, actorID := resolveActor(ctx)
	if err := logActivity(ctx, s.queries, ActivityParams{
		SquadID:    uuid.Nil,
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "secrets.master_key_rotated",
		EntityType: "system",
		EntityID:   uuid.Nil,
		Metadata:   map[string]any{"rotated_count": len(allSecrets)},
	}); err != nil {
		slog.Error("failed to log master key rotation activity", "error", err)
	}

	return len(allSecrets), nil
}

// ServiceError is a structured error with HTTP status code.
type ServiceError struct {
	Code      int
	ErrorCode string
	Message   string
}

func (e *ServiceError) Error() string {
	return e.Message
}

// secretToResponse converts a db.SquadSecret to a SecretResponse.
func secretToResponse(s db.SquadSecret) *SecretResponse {
	resp := &SecretResponse{
		ID:        s.ID,
		SquadID:   s.SquadID,
		Name:      s.Name,
		MaskedValue: s.MaskedHint,
		CreatedAt: s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if s.LastRotatedAt.Valid {
		ts := s.LastRotatedAt.Time.Format("2006-01-02T15:04:05Z")
		resp.LastRotatedAt = &ts
	}
	return resp
}

// resolveActor extracts actor type and ID from context.
func resolveActor(ctx context.Context) (domain.ActivityActorType, uuid.UUID) {
	if user, ok := auth.UserFromContext(ctx); ok {
		return domain.ActivityActorUser, user.UserID
	}
	if agent, ok := auth.AgentFromContext(ctx); ok {
		return domain.ActivityActorAgent, agent.AgentID
	}
	return domain.ActivityActorSystem, uuid.Nil
}

// isDuplicateKeyError checks if the error is a PostgreSQL unique violation.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	return containsString(err.Error(), "duplicate key") || containsString(err.Error(), "unique constraint")
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
