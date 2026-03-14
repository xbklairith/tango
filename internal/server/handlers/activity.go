package handlers

import (
	"context"
	"encoding/json"
	"sort"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// ActivityParams carries the fields needed for a single activity entry.
// Metadata is any JSON-serialisable value; pass nil for an empty object.
type ActivityParams struct {
	SquadID    uuid.UUID
	ActorType  domain.ActivityActorType
	ActorID    uuid.UUID
	Action     string
	EntityType string
	EntityID   uuid.UUID
	Metadata   any
}

// logActivity inserts a single activity entry using qtx (a transaction-scoped Queries).
// It MUST be called before tx.Commit(). Any error must be handled by the caller —
// the deferred tx.Rollback() will undo the enclosing mutation.
func logActivity(ctx context.Context, qtx *db.Queries, p ActivityParams) error {
	raw := json.RawMessage(`{}`)
	if p.Metadata != nil {
		b, err := json.Marshal(p.Metadata)
		if err != nil {
			return err
		}
		raw = b
	}

	_, err := qtx.InsertActivityEntry(ctx, db.InsertActivityEntryParams{
		SquadID:    p.SquadID,
		ActorType:  db.ActivityActorType(p.ActorType),
		ActorID:    p.ActorID,
		Action:     p.Action,
		EntityType: p.EntityType,
		EntityID:   p.EntityID,
		Metadata:   raw,
	})
	return err
}

// changedFieldNames extracts the keys from a raw JSON body map, sorts them
// alphabetically, and excludes any keys in the exclude list.
func changedFieldNames(rawBody map[string]json.RawMessage, exclude ...string) []string {
	excl := make(map[string]bool, len(exclude))
	for _, k := range exclude {
		excl[k] = true
	}
	var keys []string
	for k := range rawBody {
		if !excl[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
