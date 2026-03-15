package auth

// Resource represents a top-level entity type.
type Resource string

const (
	ResourceSquad        Resource = "squad"
	ResourceAgent        Resource = "agent"
	ResourceIssue        Resource = "issue"
	ResourceProject      Resource = "project"
	ResourceGoal         Resource = "goal"
	ResourcePipeline     Resource = "pipeline"
	ResourceInbox        Resource = "inbox"
	ResourceConversation Resource = "conversation"
	ResourceActivity     Resource = "activity"
	ResourceCost         Resource = "cost"
	ResourceTask         Resource = "task"
	ResourceRun          Resource = "run"
	ResourceWakeup       Resource = "wakeup"
	ResourceSecret       Resource = "secret"
)

// AllResources lists every defined resource constant.
var AllResources = []Resource{
	ResourceSquad, ResourceAgent, ResourceIssue, ResourceProject,
	ResourceGoal, ResourcePipeline, ResourceInbox, ResourceConversation,
	ResourceActivity, ResourceCost, ResourceTask, ResourceRun,
	ResourceWakeup, ResourceSecret,
}

// Action represents an operation on a resource.
type Action string

const (
	ActionCreate  Action = "create"
	ActionRead    Action = "read"
	ActionUpdate  Action = "update"
	ActionDelete  Action = "delete"
	ActionAssign  Action = "assign"
	ActionAdvance Action = "advance"
	ActionReject  Action = "reject"
	ActionResolve Action = "resolve"
)

// AllActions lists every defined action constant.
var AllActions = []Action{
	ActionCreate, ActionRead, ActionUpdate, ActionDelete,
	ActionAssign, ActionAdvance, ActionReject, ActionResolve,
}

// PermissionSet maps resources to their allowed actions.
type PermissionSet map[Resource]map[Action]bool

// RolePermissions maps role names to their permission sets.
type RolePermissions map[string]PermissionSet

// allActions returns a map granting all standard actions.
func allActions() map[Action]bool {
	return map[Action]bool{
		ActionCreate: true, ActionRead: true,
		ActionUpdate: true, ActionDelete: true,
		ActionAssign: true, ActionAdvance: true,
		ActionReject: true, ActionResolve: true,
	}
}

// readOnly returns a map granting only read access.
func readOnly() map[Action]bool {
	return map[Action]bool{ActionRead: true}
}

// actions builds a map from a variadic list of actions.
func actions(acts ...Action) map[Action]bool {
	m := make(map[Action]bool, len(acts))
	for _, a := range acts {
		m[a] = true
	}
	return m
}

// UserPermissions is the static user role permission matrix.
var UserPermissions = RolePermissions{
	"owner": {
		ResourceSquad:        allActions(),
		ResourceAgent:        allActions(),
		ResourceIssue:        allActions(),
		ResourceProject:      allActions(),
		ResourceGoal:         allActions(),
		ResourcePipeline:     allActions(),
		ResourceInbox:        allActions(),
		ResourceConversation: allActions(),
		ResourceActivity:     readOnly(),
		ResourceCost:         allActions(),
		ResourceTask:         allActions(),
		ResourceRun:          allActions(),
		ResourceWakeup:       allActions(),
		ResourceSecret:       allActions(),
	},
	"admin": {
		ResourceSquad:        actions(ActionCreate, ActionRead, ActionUpdate),
		ResourceAgent:        allActions(),
		ResourceIssue:        allActions(),
		ResourceProject:      allActions(),
		ResourceGoal:         allActions(),
		ResourcePipeline:     allActions(),
		ResourceInbox:        allActions(),
		ResourceConversation: allActions(),
		ResourceActivity:     readOnly(),
		ResourceCost:         allActions(),
		ResourceTask:         allActions(),
		ResourceRun:          allActions(),
		ResourceWakeup:       allActions(),
		ResourceSecret:       allActions(),
	},
	"viewer": {
		ResourceSquad:        readOnly(),
		ResourceAgent:        readOnly(),
		ResourceIssue:        readOnly(),
		ResourceProject:      readOnly(),
		ResourceGoal:         readOnly(),
		ResourcePipeline:     readOnly(),
		ResourceInbox:        readOnly(),
		ResourceConversation: readOnly(),
		ResourceActivity:     readOnly(),
		ResourceCost:         readOnly(),
		ResourceTask:         readOnly(),
		ResourceRun:          readOnly(),
		ResourceWakeup:       readOnly(),
	},
}

// AgentPermissions is the static agent role permission matrix.
var AgentPermissions = RolePermissions{
	"captain": {
		ResourceIssue:        actions(ActionCreate, ActionRead, ActionUpdate, ActionAssign, ActionAdvance, ActionReject, ActionResolve),
		ResourceAgent:        readOnly(),
		ResourceProject:      readOnly(),
		ResourceGoal:         readOnly(),
		ResourcePipeline:     readOnly(),
		ResourceInbox:        actions(ActionCreate, ActionRead, ActionResolve),
		ResourceConversation: actions(ActionCreate, ActionRead, ActionUpdate),
		ResourceActivity:     readOnly(),
		ResourceCost:         readOnly(),
		ResourceTask:         actions(ActionRead, ActionUpdate),
		ResourceRun:          readOnly(),
		ResourceWakeup:       actions(ActionCreate),
	},
	"lead": {
		ResourceIssue:        actions(ActionCreate, ActionRead, ActionUpdate, ActionAssign, ActionAdvance, ActionReject, ActionResolve),
		ResourceAgent:        readOnly(),
		ResourceProject:      readOnly(),
		ResourceGoal:         readOnly(),
		ResourcePipeline:     readOnly(),
		ResourceInbox:        actions(ActionCreate, ActionRead),
		ResourceConversation: actions(ActionCreate, ActionRead, ActionUpdate),
		ResourceActivity:     readOnly(),
		ResourceCost:         readOnly(),
		ResourceTask:         actions(ActionRead, ActionUpdate),
		ResourceRun:          readOnly(),
		ResourceWakeup:       actions(ActionCreate),
	},
	"member": {
		ResourceIssue:        actions(ActionRead, ActionUpdate),
		ResourceAgent:        readOnly(),
		ResourceProject:      readOnly(),
		ResourceGoal:         readOnly(),
		ResourcePipeline:     readOnly(),
		ResourceInbox:        actions(ActionCreate, ActionRead),
		ResourceConversation: actions(ActionCreate, ActionRead),
		ResourceActivity:     readOnly(),
		ResourceCost:         readOnly(),
		ResourceTask:         actions(ActionRead, ActionUpdate),
		ResourceRun:          readOnly(),
	},
}
