// Package auth provides authentication, authorization, and session management.
package auth

import "github.com/google/uuid"

// DeploymentMode determines the authentication behavior of the server.
type DeploymentMode string

const (
	ModeLocalTrusted  DeploymentMode = "local_trusted"
	ModeAuthenticated DeploymentMode = "authenticated"
)

// Identity represents the authenticated user injected into the request context.
type Identity struct {
	UserID uuid.UUID
	Email  string
}

// LocalOperatorIdentity is the synthetic identity used in local_trusted mode.
var LocalOperatorIdentity = Identity{
	UserID: uuid.Nil,
	Email:  "local@ari.local",
}
