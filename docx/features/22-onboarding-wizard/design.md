# Onboarding Wizard — Technical Design

## Architecture Overview

The onboarding wizard is a **frontend-driven** feature with minimal backend changes. The core idea: accumulate all user input across 4 wizard steps, then create resources using existing (slightly extended) APIs.

```
┌─────────────────────────────────────────────────┐
│  OnboardingWizard (full-screen)                 │
│                                                 │
│  ┌─────────┐  ┌──────────┐  ┌───────┐  ┌─────┐│
│  │ Step 1  │→ │ Step 2   │→ │Step 3 │→ │ S4  ││
│  │ Squad   │  │ Captain  │  │ Task  │  │Done ││
│  │ Details │  │ +Adapter │  │(skip) │  │     ││
│  └─────────┘  └──────────┘  └───────┘  └─────┘│
│       ↓            ↓             ↓              │
│  wizard state  wizard state  wizard state       │
│       └────────────┴─────────────┘              │
│                    ↓                            │
│            POST /api/squads (step 2 complete)   │
│            POST /api/issues (step 3 complete)   │
└─────────────────────────────────────────────────┘
```

**Key decision:** Squad + captain are created atomically at Step 2 completion (not Step 1). This matches the existing backend pattern and avoids orphaned squads.

## Component Structure

```
web/src/features/onboarding/
├── onboarding-wizard.tsx          # Full-screen wizard container + step state
├── step-squad-details.tsx         # Step 1: Squad name, prefix, description
├── step-captain-config.tsx        # Step 2: Captain name, adapter type, config
├── step-first-task.tsx            # Step 3: Bootstrap task (skippable)
├── step-launch.tsx                # Step 4: Summary + navigate
├── adapter-config-fields.tsx      # Adapter-specific form fields (claude_local, process)
└── use-adapter-test.ts            # Hook: POST /api/adapters/{type}/test-environment
```

### Component Responsibilities

**OnboardingWizard** — Top-level container
- Manages wizard state (all form data across steps)
- Tracks current step (1-4)
- Handles forward/back navigation
- Holds created resource IDs (squadId, captainId)
- Full-screen layout: left half = form, right half = decorative branding or step illustration

**StepSquadDetails** — Step 1
- Inputs: name, issuePrefix, description
- Validation: same rules as current Create Squad dialog
- Does NOT call any API — stores in wizard state
- Advance: sets step to 2

**StepCaptainConfig** — Step 2
- Inputs: captainName, captainShortName, adapterType, adapterConfig fields
- Fetches available adapters from `GET /api/adapters`
- Fetches models from `GET /api/adapters/{type}/models`
- "Test Environment" button: `POST /api/adapters/{type}/test-environment`
- On "Next": calls `POST /api/squads` with all squad + captain data
- Stores returned squadId and captainId in wizard state

**StepFirstTask** — Step 3
- Inputs: title (pre-filled), description (pre-filled with bootstrap instructions)
- "Skip" button advances to Step 4 without creating an issue
- On "Next": calls `POST /api/issues` with task assigned to captain

**StepLaunch** — Step 4
- Shows summary: squad name, captain name, adapter type, task (if created)
- "Go to Dashboard" button: navigates to `/`

**AdapterConfigFields** — Conditional sub-form
- Renders adapter-specific fields based on selected adapter type
- For `claude_local`: workingDir, model dropdown, collapsible advanced (timeout, skipPermissions, env)
- For `process`: command, args, workingDir
- Extensible pattern for future adapter types

## Data Flow

### Step-by-step resource creation

```
Step 1 → Step 2 (no API calls, data in state)

Step 2 → "Next" clicked:
  POST /api/squads
  Body: {
    name, issuePrefix, description,
    captainName, captainShortName,
    captainAdapterType: "claude_local",
    captainAdapterConfig: {
      "workingDir": "/Users/xb/project",
      "model": "sonnet",
      "skipPermissions": true
    }
  }
  Response: { id, name, slug, ... }
  → Store squadId in wizard state
  → Fetch captain via GET /api/agents?squadId={id} to get captainId
  → Advance to Step 3

Step 3 → "Next" clicked (or "Skip"):
  POST /api/issues
  Body: {
    squadId, title, description,
    assigneeAgentId: captainId,
    status: "todo"
  }
  → Advance to Step 4

Step 4 → "Go to Dashboard":
  → Set active squad
  → Navigate to /
```

### Wizard State Shape

```typescript
interface WizardState {
  // Step 1
  squadName: string;
  issuePrefix: string;
  description: string;

  // Step 2
  captainName: string;        // default: "Captain"
  captainShortName: string;   // auto-derived from name
  adapterType: string;        // default: first available
  adapterConfig: {
    workingDir: string;
    model: string;
    skipPermissions: boolean;
    timeoutSeconds: number;
    // ... adapter-specific fields
  };

  // Step 3
  taskTitle: string;           // pre-filled
  taskDescription: string;     // pre-filled

  // Created resource IDs
  createdSquadId: string | null;
  createdCaptainId: string | null;
  createdIssueId: string | null;
}
```

## API Contracts

### Modified: POST /api/squads

Add 2 optional fields to `createSquadRequest`:

```go
type createSquadRequest struct {
    // ... existing fields ...
    CaptainName      string `json:"captainName"`
    CaptainShortName string `json:"captainShortName"`

    // NEW: optional adapter config for captain
    CaptainAdapterType   *string                `json:"captainAdapterType,omitempty"`
    CaptainAdapterConfig map[string]interface{} `json:"captainAdapterConfig,omitempty"`
}
```

**Behavior:**
- If `captainAdapterType` is provided → use it instead of hardcoded `claude_local`
- If `captainAdapterConfig` is provided → store as agent's `adapter_config` JSONB
- If omitted → keep current defaults (claude_local, empty config)
- Existing clients are unaffected (fields are optional)

### New: GET /api/adapters

List available adapters from the registry.

```
GET /api/adapters
Response 200:
[
  {
    "type": "claude_local",
    "available": true,
    "models": [
      {"id": "sonnet", "name": "Claude Sonnet", "provider": "anthropic"},
      {"id": "opus", "name": "Claude Opus", "provider": "anthropic"},
      {"id": "haiku", "name": "Claude Haiku", "provider": "anthropic"}
    ]
  },
  {
    "type": "process",
    "available": true,
    "models": []
  }
]
```

**Backend:** Add `ListAvailable() []AdapterInfo` to the Registry. Returns type, availability status, and models for each registered adapter.

### New: POST /api/adapters/{type}/test-environment

Invoke the adapter's `TestEnvironment()` method.

```
POST /api/adapters/claude_local/test-environment
Request: {} (no body needed for now)

Response 200:
{
  "available": true,
  "message": "Claude CLI found at /usr/local/bin/claude, version 1.2.3"
}

Response 200 (failure):
{
  "available": false,
  "message": "Claude CLI not found in PATH"
}
```

**Auth:** Requires authenticated user. No squad scope needed (adapter is machine-level).

### New: GET /api/adapters/{type}/models

Return models supported by an adapter.

```
GET /api/adapters/claude_local/models

Response 200:
[
  {"id": "sonnet", "name": "Claude Sonnet", "provider": "anthropic"},
  {"id": "opus", "name": "Claude Opus", "provider": "anthropic"},
  {"id": "haiku", "name": "Claude Haiku", "provider": "anthropic"}
]
```

## Backend Changes

### 1. Adapter Registry — Add ListAvailable()

```go
// internal/adapter/registry.go

type AdapterInfo struct {
    Type      string            `json:"type"`
    Available bool              `json:"available"`
    Reason    string            `json:"reason,omitempty"`
    Models    []ModelDefinition `json:"models"`
}

func (r *Registry) ListAvailable() []AdapterInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()

    var result []AdapterInfo
    for typ, a := range r.adapters {
        info := AdapterInfo{
            Type:      typ,
            Available: true,
            Models:    a.Models(),
        }
        if reason, ok := r.unavailable[typ]; ok {
            info.Available = false
            info.Reason = reason
        }
        result = append(result, info)
    }
    return result
}
```

### 2. Adapter Handler — New handler for adapter endpoints

```go
// internal/server/handlers/adapter_handler.go

type AdapterHandler struct {
    registry *adapter.Registry
}

func (h *AdapterHandler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/adapters", h.List)
    mux.HandleFunc("GET /api/adapters/{type}/models", h.ListModels)
    mux.HandleFunc("POST /api/adapters/{type}/test-environment", h.TestEnvironment)
}
```

### 3. Squad Handler — Accept captain adapter config

Extend `createSquadRequest` with optional `CaptainAdapterType` and `CaptainAdapterConfig` fields. In the captain creation logic, use provided values instead of hardcoding.

```go
// In squad_handler.go Create method, captain creation section:

adapterType := db.AdapterTypeClaudeLocal // default
if req.CaptainAdapterType != nil {
    adapterType = db.AdapterType(*req.CaptainAdapterType)
}

var adapterConfigJSON json.RawMessage
if req.CaptainAdapterConfig != nil {
    adapterConfigJSON, _ = json.Marshal(req.CaptainAdapterConfig)
}
```

## Frontend Changes

### 1. New OnboardingWizard component

Full-screen component that replaces the Create Squad dialog for first-time users.

**Layout:**
```
┌──────────────────────┬──────────────────────┐
│                      │                      │
│   Close (X)          │                      │
│                      │                      │
│   ✦ Get Started      │    (right panel:     │
│   Step 2 of 4        │     step-specific    │
│   ●● ○ ○             │     illustration     │
│                      │     or branding)     │
│   [Form fields]      │                      │
│                      │                      │
│                      │                      │
│   [Back] [Next]      │                      │
│                      │                      │
└──────────────────────┴──────────────────────┘
```

### 2. Modify Dashboard empty state

```tsx
// dashboard-page.tsx — replace CreateSquadDialog with OnboardingWizard
if (!activeSquad) {
  return <OnboardingWizard onComplete={(squadId) => {
    setActiveSquadId(squadId);
    // navigate handled by wizard
  }} />;
}
```

**Key change:** The empty state no longer shows a CTA button + dialog. Instead, the wizard IS the empty state — shown inline or full-screen immediately.

### 3. Keep existing Create Squad dialog

For users who already have squads and want to create additional ones, the existing dialog remains unchanged.

### 4. New API hooks

```typescript
// web/src/api/adapters.ts
export const adaptersApi = {
  list: () => get<AdapterInfo[]>('/api/adapters'),
  models: (type: string) => get<ModelDefinition[]>(`/api/adapters/${type}/models`),
  testEnvironment: (type: string) => post<TestResult>(`/api/adapters/${type}/test-environment`),
};
```

## Error Handling

| Scenario | Handling |
|----------|----------|
| Squad creation fails (Step 2) | Show error inline, keep form data, allow retry |
| Adapter test fails (Step 2) | Show warning with details, allow "proceed anyway" |
| Issue creation fails (Step 3) | Show error inline, allow retry or skip |
| Network error | Show generic error banner with retry button |
| User closes wizard mid-flow | If squad created (past step 2): squad persists, user can configure later. If before step 2: nothing was created, clean exit |

## Security Considerations

- Adapter test endpoint requires authenticated user (JWT)
- Adapter config is stored as JSONB — no secrets in v0.1 (future: REQ from feature 19)
- `workingDir` validation: must be an absolute path, no traversal (backend validation)
- `adapterType` validated against registry — rejects unknown types

## Testing Strategy

### Backend Unit Tests
- AdapterHandler: list adapters, list models, test environment
- Squad handler: create squad with captain adapter config (new fields)
- Registry: ListAvailable() returns correct info

### Backend Integration Tests
- POST /api/squads with captainAdapterType + captainAdapterConfig → captain created with config
- GET /api/adapters → returns available adapters
- POST /api/adapters/claude_local/test-environment → returns test result

### Frontend Component Tests
- OnboardingWizard: step navigation, back/forward, state preservation
- StepCaptainConfig: adapter selection, model dropdown, test button
- AdapterConfigFields: renders correct fields per adapter type

### E2E Tests
- Full wizard flow: no squads → wizard → create squad + captain with adapter → create task → dashboard
- Wizard with skip task: no squads → wizard → create squad + captain → skip task → dashboard
- Wizard close: close mid-flow → verify no orphaned resources
