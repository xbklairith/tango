package domain

import (
	"time"

	"github.com/google/uuid"
)

// ActivityActorType identifies who triggered the mutation.
type ActivityActorType string

const (
	ActivityActorUser   ActivityActorType = "user"
	ActivityActorAgent  ActivityActorType = "agent"
	ActivityActorSystem ActivityActorType = "system"
)

func (a ActivityActorType) Valid() bool {
	switch a {
	case ActivityActorUser, ActivityActorAgent, ActivityActorSystem:
		return true
	}
	return false
}

// ValidActivityEntityTypes is the controlled list for the entityType field.
var ValidActivityEntityTypes = map[string]bool{
	"squad": true, "agent": true, "issue": true,
	"comment": true, "project": true, "goal": true, "member": true,
	"inbox_item": true,
}

// ActivityEntry is the domain model returned from the feed endpoint.
type ActivityEntry struct {
	ID         uuid.UUID         `json:"id"`
	SquadID    uuid.UUID         `json:"squadId"`
	ActorType  ActivityActorType `json:"actorType"`
	ActorID    uuid.UUID         `json:"actorId"`
	Action     string            `json:"action"`
	EntityType string            `json:"entityType"`
	EntityID   uuid.UUID         `json:"entityId"`
	Metadata   any               `json:"metadata"`
	CreatedAt  time.Time         `json:"createdAt"`
}
