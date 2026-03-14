# Design: React UI Foundation

**Created:** 2026-03-14
**Status:** Draft
**Feature:** 07-react-ui-foundation
**Dependencies:** 01-go-scaffold, 02-user-auth, 03-squad-management, 04-agent-management, 05-issue-tracking, 06-projects-goals

---

## 1. Architecture Overview

The React UI Foundation is a single-page application (SPA) built with React 19, Vite, Tailwind CSS 4, and shadcn/ui. The production build (`web/dist/`) is embedded into the Go binary via `go:embed`, allowing `ari run` to serve both the API and the frontend from a single process with zero external dependencies. In development, Vite's dev server runs independently with HMR and proxies API requests to the Go backend at `http://localhost:3100`.

The frontend uses a feature-based directory structure under `web/src/features/`, with shared layout components, a typed API client built on `fetch`, TanStack Query for server state management, and React Router v7 for client-side routing. Authentication is handled via HTTP-only session cookies set by the Go backend; the frontend maintains an auth context but never touches tokens directly.

```
Browser
  │
  ├── Development: Vite Dev Server (:5173) ──proxy──> Go API (:3100/api/*)
  │
  └── Production:  Go Binary (:3100)
                     ├── /api/*     → API handlers
                     └── /*         → go:embed SPA (index.html fallback)
```

---

## 2. System Context

- **Depends On:**
  - Go HTTP server (01-go-scaffold) for API endpoints and static file serving
  - Auth API (02-user-auth) for login/logout/session validation
  - Squad API (03-squad-management) for squad CRUD and membership
  - Agent API (04-agent-management) for agent CRUD and hierarchy
  - Issue API (05-issue-tracking) for issue/comment CRUD
  - Projects & Goals API (06-projects-goals) for project/goal CRUD
- **Used By:** End users (human operators) via web browser
- **External Dependencies:** npm packages (React, Vite, TanStack Query, React Router, shadcn/ui, Radix UI, Tailwind CSS)

---

## 3. Component Structure

### 3.1 Directory Layout

```
web/
├── index.html                    # Vite HTML entry
├── package.json
├── tsconfig.json
├── vite.config.ts
├── tailwind.config.ts            # Tailwind CSS 4 config (if needed beyond CSS)
├── components.json               # shadcn/ui configuration
├── public/
│   └── favicon.svg
├── src/
│   ├── main.tsx                  # React DOM root mount
│   ├── app.tsx                   # Router setup, providers, error boundary
│   ├── globals.css               # Tailwind directives, CSS variables, theme
│   │
│   ├── lib/
│   │   ├── api.ts                # Typed fetch wrapper
│   │   ├── auth.tsx              # Auth context, AuthGuard, useAuth hook
│   │   ├── query.ts              # TanStack Query client config
│   │   └── utils.ts              # cn() helper, formatters
│   │
│   ├── types/
│   │   ├── api.ts                # Shared API response/error types
│   │   ├── squad.ts              # Squad, SquadMembership types
│   │   ├── agent.ts              # Agent types, status/role enums
│   │   ├── issue.ts              # Issue, IssueComment types, status enums
│   │   ├── project.ts            # Project types
│   │   ├── goal.ts               # Goal types
│   │   └── user.ts               # User, AuthUser types
│   │
│   ├── components/
│   │   ├── ui/                   # shadcn/ui components (auto-generated)
│   │   │   ├── button.tsx
│   │   │   ├── input.tsx
│   │   │   ├── card.tsx
│   │   │   ├── table.tsx
│   │   │   ├── dialog.tsx
│   │   │   ├── dropdown-menu.tsx
│   │   │   ├── select.tsx
│   │   │   ├── badge.tsx
│   │   │   ├── tabs.tsx
│   │   │   ├── toast.tsx
│   │   │   ├── skeleton.tsx
│   │   │   ├── avatar.tsx
│   │   │   ├── separator.tsx
│   │   │   ├── sheet.tsx
│   │   │   ├── command.tsx
│   │   │   ├── popover.tsx
│   │   │   ├── form.tsx
│   │   │   └── label.tsx
│   │   │
│   │   └── layout/
│   │       ├── app-layout.tsx     # Sidebar + header + content wrapper
│   │       ├── sidebar.tsx        # Navigation sidebar
│   │       ├── header.tsx         # Page header with breadcrumbs
│   │       ├── breadcrumbs.tsx    # Breadcrumb trail component
│   │       ├── loading-screen.tsx # Full-page loading indicator
│   │       └── error-boundary.tsx # React error boundary with retry
│   │
│   ├── features/
│   │   ├── auth/
│   │   │   └── login-page.tsx
│   │   │
│   │   ├── dashboard/
│   │   │   ├── dashboard-page.tsx
│   │   │   ├── stats-cards.tsx
│   │   │   ├── recent-activity.tsx
│   │   │   └── quick-actions.tsx
│   │   │
│   │   ├── squads/
│   │   │   ├── squad-list-page.tsx
│   │   │   ├── squad-detail-page.tsx
│   │   │   ├── squad-form.tsx
│   │   │   ├── squad-members.tsx
│   │   │   └── hooks.ts           # useSquads, useSquad, useCreateSquad, etc.
│   │   │
│   │   ├── agents/
│   │   │   ├── agent-list-page.tsx
│   │   │   ├── agent-detail-page.tsx
│   │   │   ├── agent-form.tsx
│   │   │   ├── agent-tree-view.tsx
│   │   │   ├── agent-status-badge.tsx
│   │   │   └── hooks.ts
│   │   │
│   │   ├── issues/
│   │   │   ├── issue-list-page.tsx
│   │   │   ├── issue-detail-page.tsx
│   │   │   ├── issue-form.tsx
│   │   │   ├── issue-comments.tsx
│   │   │   ├── issue-status-select.tsx
│   │   │   ├── issue-filters.tsx
│   │   │   └── hooks.ts
│   │   │
│   │   ├── projects/
│   │   │   ├── project-list-page.tsx
│   │   │   ├── project-detail-page.tsx
│   │   │   ├── project-form.tsx
│   │   │   └── hooks.ts
│   │   │
│   │   └── goals/
│   │       ├── goal-list-page.tsx
│   │       ├── goal-detail-page.tsx
│   │       ├── goal-form.tsx
│   │       └── hooks.ts
│   │
│   └── hooks/
│       ├── use-toast.ts           # Toast notification hook
│       └── use-debounce.ts        # Input debounce utility
│
├── tests/
│   ├── setup.ts                   # Vitest setup (jsdom, mocks)
│   ├── lib/
│   │   └── api.test.ts
│   └── features/
│       ├── auth/
│       │   └── login-page.test.tsx
│       └── ...
│
└── e2e/
    ├── playwright.config.ts
    ├── auth.spec.ts
    ├── dashboard.spec.ts
    └── ...
```

### 3.2 Entry Point: `web/src/main.tsx`

```tsx
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app";
import "./globals.css";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>
);
```

### 3.3 App Shell: `web/src/app.tsx`

```tsx
import { BrowserRouter, Routes, Route } from "react-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/query";
import { AuthProvider, AuthGuard } from "./lib/auth";
import { AppLayout } from "./components/layout/app-layout";
import { ErrorBoundary } from "./components/layout/error-boundary";
import { Toaster } from "./components/ui/toast";
import { lazy, Suspense } from "react";
import { LoadingScreen } from "./components/layout/loading-screen";

// Lazy-loaded page components for code splitting
const LoginPage = lazy(() => import("./features/auth/login-page"));
const DashboardPage = lazy(() => import("./features/dashboard/dashboard-page"));
const SquadListPage = lazy(() => import("./features/squads/squad-list-page"));
const SquadDetailPage = lazy(() => import("./features/squads/squad-detail-page"));
const AgentListPage = lazy(() => import("./features/agents/agent-list-page"));
const AgentDetailPage = lazy(() => import("./features/agents/agent-detail-page"));
const IssueListPage = lazy(() => import("./features/issues/issue-list-page"));
const IssueDetailPage = lazy(() => import("./features/issues/issue-detail-page"));
const ProjectListPage = lazy(() => import("./features/projects/project-list-page"));
const ProjectDetailPage = lazy(() => import("./features/projects/project-detail-page"));
const GoalListPage = lazy(() => import("./features/goals/goal-list-page"));
const GoalDetailPage = lazy(() => import("./features/goals/goal-detail-page"));
const NotFoundPage = lazy(() => import("./features/not-found-page"));

export function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <AuthProvider>
            <Toaster />
            <Routes>
              {/* Public routes */}
              <Route path="/login" element={
                <Suspense fallback={<LoadingScreen />}>
                  <LoginPage />
                </Suspense>
              } />

              {/* Protected routes */}
              <Route element={<AuthGuard />}>
                <Route element={<AppLayout />}>
                  <Route index element={
                    <Suspense fallback={<LoadingScreen />}>
                      <DashboardPage />
                    </Suspense>
                  } />
                  <Route path="squads" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <SquadListPage />
                    </Suspense>
                  } />
                  <Route path="squads/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <SquadDetailPage />
                    </Suspense>
                  } />
                  <Route path="squads/:id/agents" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <AgentListPage />
                    </Suspense>
                  } />
                  <Route path="agents/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <AgentDetailPage />
                    </Suspense>
                  } />
                  <Route path="squads/:id/issues" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <IssueListPage />
                    </Suspense>
                  } />
                  <Route path="issues/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <IssueDetailPage />
                    </Suspense>
                  } />
                  <Route path="squads/:id/projects" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <ProjectListPage />
                    </Suspense>
                  } />
                  <Route path="projects/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <ProjectDetailPage />
                    </Suspense>
                  } />
                  <Route path="squads/:id/goals" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <GoalListPage />
                    </Suspense>
                  } />
                  <Route path="goals/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <GoalDetailPage />
                    </Suspense>
                  } />
                  <Route path="*" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <NotFoundPage />
                    </Suspense>
                  } />
                </Route>
              </Route>
            </Routes>
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}
```

---

## 4. Data Flow

### 4.1 TanStack Query Configuration

```typescript
// web/src/lib/query.ts
import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,           // 30s before data is considered stale
      gcTime: 5 * 60_000,         // 5min garbage collection
      retry: 1,                    // Retry failed requests once
      refetchOnWindowFocus: true,  // Refetch when tab regains focus
    },
    mutations: {
      retry: 0,                    // No retries for mutations
    },
  },
});
```

### 4.2 Query Key Convention

All query keys follow a hierarchical tuple pattern for precise cache invalidation:

```typescript
// Query key factory pattern
export const queryKeys = {
  squads: {
    all: ["squads"] as const,
    detail: (id: string) => ["squads", id] as const,
    members: (id: string) => ["squads", id, "members"] as const,
  },
  agents: {
    list: (squadId: string) => ["agents", { squadId }] as const,
    detail: (id: string) => ["agents", id] as const,
    tree: (squadId: string) => ["agents", "tree", { squadId }] as const,
  },
  issues: {
    list: (squadId: string, filters?: IssueFilters) =>
      ["issues", { squadId, ...filters }] as const,
    detail: (id: string) => ["issues", id] as const,
    comments: (issueId: string) => ["issues", id, "comments"] as const,
  },
  projects: {
    list: (squadId: string) => ["projects", { squadId }] as const,
    detail: (id: string) => ["projects", id] as const,
  },
  goals: {
    list: (squadId: string) => ["goals", { squadId }] as const,
    detail: (id: string) => ["goals", id] as const,
  },
  auth: {
    me: ["auth", "me"] as const,
  },
} as const;
```

### 4.3 Cache Invalidation Strategy

| Mutation | Invalidates |
|----------|------------|
| Create squad | `queryKeys.squads.all` |
| Update squad | `queryKeys.squads.detail(id)`, `queryKeys.squads.all` |
| Create agent | `queryKeys.agents.list(squadId)`, `queryKeys.agents.tree(squadId)` |
| Update agent | `queryKeys.agents.detail(id)`, `queryKeys.agents.list(squadId)`, `queryKeys.agents.tree(squadId)` |
| Create issue | `queryKeys.issues.list(squadId)` |
| Update issue | `queryKeys.issues.detail(id)`, `queryKeys.issues.list(squadId)` |
| Add comment | `queryKeys.issues.comments(issueId)` |
| Create project | `queryKeys.projects.list(squadId)` |
| Update project | `queryKeys.projects.detail(id)`, `queryKeys.projects.list(squadId)` |
| Create goal | `queryKeys.goals.list(squadId)` |
| Update goal | `queryKeys.goals.detail(id)`, `queryKeys.goals.list(squadId)` |

### 4.4 Data Flow Diagram

```
User Interaction
      │
      ▼
React Component (page/form)
      │
      ├── Read: useQuery(queryKey, apiFn)
      │         │
      │         ▼
      │   TanStack Query Cache
      │         │ (cache miss or stale)
      │         ▼
      │   api.get/post/patch()  ──► Go Backend (/api/*)
      │         │                        │
      │         ◄────────────────────────┘
      │         │
      │         ▼
      │   Cache updated, component re-renders
      │
      └── Write: useMutation(apiFn, { onSuccess: invalidate })
                │
                ▼
          api.post/patch/delete() ──► Go Backend
                │
                ▼
          onSuccess: queryClient.invalidateQueries(keys)
                │
                ▼
          Stale queries refetch automatically
```

---

## 5. Routing

### 5.1 Full Route Table

| Route | Component | Auth | Description |
|-------|-----------|------|-------------|
| `/login` | `LoginPage` | Public | Login form |
| `/` | `DashboardPage` | Protected | Squad overview dashboard |
| `/squads` | `SquadListPage` | Protected | List user's squads |
| `/squads/:id` | `SquadDetailPage` | Protected | Squad settings, members |
| `/squads/:id/agents` | `AgentListPage` | Protected | Agents in a squad |
| `/agents/:id` | `AgentDetailPage` | Protected | Agent profile, hierarchy |
| `/squads/:id/issues` | `IssueListPage` | Protected | Issues in a squad |
| `/issues/:id` | `IssueDetailPage` | Protected | Issue detail, comments |
| `/squads/:id/projects` | `ProjectListPage` | Protected | Projects in a squad |
| `/projects/:id` | `ProjectDetailPage` | Protected | Project detail, linked issues |
| `/squads/:id/goals` | `GoalListPage` | Protected | Goals in a squad |
| `/goals/:id` | `GoalDetailPage` | Protected | Goal detail, linked projects |
| `*` | `NotFoundPage` | Protected | 404 catch-all |

### 5.2 AuthGuard Component

The `AuthGuard` acts as a layout route that blocks rendering of child routes until auth state is resolved.

```tsx
// web/src/lib/auth.tsx
import { Navigate, Outlet, useLocation } from "react-router";
import { useAuth } from "./auth";
import { LoadingScreen } from "../components/layout/loading-screen";

export function AuthGuard() {
  const { user, isLoading } = useAuth();
  const location = useLocation();

  if (isLoading) {
    return <LoadingScreen />;
  }

  if (!user) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  return <Outlet />;
}
```

### 5.3 Breadcrumb Mapping

Breadcrumbs are derived from the current URL path and route parameters:

```typescript
// Route-to-breadcrumb mapping
const breadcrumbMap: Record<string, string | ((params: Params) => string)> = {
  "/": "Dashboard",
  "/squads": "Squads",
  "/squads/:id": (params) => squadName ?? "Squad",  // resolved from cache
  "/squads/:id/agents": "Agents",
  "/agents/:id": (params) => agentName ?? "Agent",
  "/squads/:id/issues": "Issues",
  "/issues/:id": (params) => issueIdentifier ?? "Issue",
  "/squads/:id/projects": "Projects",
  "/projects/:id": (params) => projectName ?? "Project",
  "/squads/:id/goals": "Goals",
  "/goals/:id": (params) => goalTitle ?? "Goal",
};
```

---

## 6. Go Embedding

### 6.1 Embedding Strategy

The Go binary embeds the Vite build output using `go:embed`. A dedicated file handler serves static assets with proper caching headers and falls back to `index.html` for all non-API, non-file routes to support SPA client-side routing.

```go
// internal/server/spa.go
package server

import (
    "embed"
    "io/fs"
    "net/http"
    "strings"
)

//go:embed all:web/dist
var webFS embed.FS

func spaHandler() http.Handler {
    // Strip the "web/dist" prefix to serve from root
    dist, _ := fs.Sub(webFS, "web/dist")
    fileServer := http.FileServer(http.FS(dist))

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Skip API routes
        if strings.HasPrefix(r.URL.Path, "/api/") {
            http.NotFound(w, r)
            return
        }

        // Try serving the exact file (JS, CSS, images, etc.)
        path := r.URL.Path
        if path == "/" {
            path = "/index.html"
        }

        // Check if file exists in embedded FS
        f, err := dist.Open(strings.TrimPrefix(path, "/"))
        if err == nil {
            f.Close()
            // Static assets with content hashes get long cache
            if strings.Contains(path, "/assets/") {
                w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
            }
            fileServer.ServeHTTP(w, r)
            return
        }

        // Fallback to index.html for SPA client-side routing
        r.URL.Path = "/"
        w.Header().Set("Cache-Control", "no-cache")
        fileServer.ServeHTTP(w, r)
    })
}
```

### 6.2 Router Integration

```go
// internal/server/router.go
func (s *Server) setupRoutes() {
    mux := http.NewServeMux()

    // API routes (handled first due to prefix match)
    mux.Handle("/api/", s.apiRouter())

    // SPA catch-all (serves embedded frontend)
    mux.Handle("/", spaHandler())

    s.handler = mux
}
```

### 6.3 Build Integration

The `Makefile` ensures the frontend is built before the Go binary:

```makefile
ui-build:
	cd web && npm run build

build: ui-build
	go build -o bin/ari ./cmd/ari

ui-dev:
	cd web && npm run dev
```

### 6.4 Vite Configuration

```typescript
// web/vite.config.ts
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:3100",
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: "dist",
    sourcemap: false,
    rollupOptions: {
      output: {
        // Content hash in filenames for cache busting
        entryFileNames: "assets/[name]-[hash].js",
        chunkFileNames: "assets/[name]-[hash].js",
        assetFileNames: "assets/[name]-[hash].[ext]",
      },
    },
  },
});
```

---

## 7. API Client

### 7.1 Core Types

```typescript
// web/src/types/api.ts

/** Standard API error response from Go backend */
export interface ApiError {
  error: string;
  code: string;
}

/** Paginated list response wrapper */
export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  offset: number;
  limit: number;
}

/** Pagination parameters */
export interface PaginationParams {
  offset?: number;
  limit?: number;
}
```

### 7.2 Typed Fetch Wrapper

```typescript
// web/src/lib/api.ts

import type { ApiError } from "@/types/api";

const API_BASE = "/api";

class ApiClientError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiClientError";
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const url = `${API_BASE}${path}`;
  const headers: Record<string, string> = {};

  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      credentials: "same-origin",  // Send ari_session cookie
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiClientError(0, "NETWORK_ERROR", "Network connection failed");
  }

  // Handle 401 globally — redirect to login
  if (response.status === 401) {
    // Clear query cache and redirect
    window.location.href = "/login";
    throw new ApiClientError(401, "UNAUTHENTICATED", "Session expired");
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  const data = await response.json();

  if (!response.ok) {
    const apiError = data as ApiError;
    throw new ApiClientError(
      response.status,
      apiError.code ?? "UNKNOWN",
      apiError.error ?? "An unexpected error occurred",
    );
  }

  return data as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  delete: <T>(path: string) => request<T>("DELETE", path),
};

export { ApiClientError };
```

### 7.3 Feature-Specific API Functions

Each feature module exports typed API functions consumed by TanStack Query hooks:

```typescript
// web/src/features/squads/hooks.ts
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Squad, CreateSquadRequest } from "@/types/squad";
import { useToast } from "@/hooks/use-toast";

export function useSquads() {
  return useQuery({
    queryKey: queryKeys.squads.all,
    queryFn: () => api.get<Squad[]>("/squads"),
  });
}

export function useSquad(id: string) {
  return useQuery({
    queryKey: queryKeys.squads.detail(id),
    queryFn: () => api.get<Squad>(`/squads/${id}`),
    enabled: !!id,
  });
}

export function useCreateSquad() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: (data: CreateSquadRequest) =>
      api.post<Squad>("/squads", data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.squads.all });
      toast({ title: "Squad created" });
    },
    onError: (error) => {
      toast({ title: "Failed to create squad", description: error.message, variant: "destructive" });
    },
  });
}
```

---

## 8. Auth Integration

### 8.1 Auth Context

```typescript
// web/src/lib/auth.tsx
import {
  createContext,
  useContext,
  type ReactNode,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { queryKeys } from "./query";
import type { AuthUser } from "@/types/user";

interface AuthContextValue {
  user: AuthUser | null;
  isLoading: boolean;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();

  const { data: user, isLoading } = useQuery({
    queryKey: queryKeys.auth.me,
    queryFn: () => api.get<AuthUser>("/auth/me"),
    retry: false,          // Don't retry on 401
    staleTime: 5 * 60_000, // 5min — session rarely changes
  });

  async function logout() {
    await api.post("/auth/logout");
    queryClient.clear();
    window.location.href = "/login";
  }

  return (
    <AuthContext.Provider value={{ user: user ?? null, isLoading, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return ctx;
}
```

### 8.2 Auth Types

```typescript
// web/src/types/user.ts

export interface AuthUser {
  id: string;
  email: string;
  displayName: string;
  status: "active" | "disabled";
  squads: AuthSquadMembership[];
}

export interface AuthSquadMembership {
  squadId: string;
  squadName: string;
  role: "owner" | "admin" | "viewer";
}
```

### 8.3 Login Page

```tsx
// web/src/features/auth/login-page.tsx
import { useState, type FormEvent } from "react";
import { useNavigate, useLocation } from "react-router";
import { api, ApiClientError } from "@/lib/api";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/query";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export default function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const from = (location.state as { from?: Location })?.from?.pathname ?? "/";

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setIsSubmitting(true);

    try {
      await api.post("/auth/login", { email, password });
      // Refetch the user profile after login
      await queryClient.invalidateQueries({ queryKey: queryKeys.auth.me });
      navigate(from, { replace: true });
    } catch (err) {
      if (err instanceof ApiClientError) {
        setError(err.message);
      } else {
        setError("An unexpected error occurred");
      }
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">Ari</CardTitle>
          <p className="text-sm text-muted-foreground">
            Sign in to continue
          </p>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            {error && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                {error}
              </div>
            )}
            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                autoComplete="email"
                autoFocus
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                autoComplete="current-password"
              />
            </div>
            <Button type="submit" className="w-full" disabled={isSubmitting}>
              {isSubmitting ? "Signing in..." : "Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
```

### 8.4 Login Flow Sequence

```
1. User opens any protected route
2. AuthGuard sees isLoading=true → shows LoadingScreen
3. AuthProvider's useQuery calls GET /api/auth/me
   a. 200 OK → user set in context → AuthGuard renders child routes
   b. 401    → user is null → AuthGuard redirects to /login
4. User submits login form → POST /api/auth/login
5. Backend sets ari_session HTTP-only cookie
6. Frontend invalidates auth.me query → refetch resolves user
7. Navigate to original route (or "/" by default)
```

---

## 9. TypeScript Domain Types

### 9.1 Squad Types

```typescript
// web/src/types/squad.ts

export interface Squad {
  id: string;
  name: string;
  issuePrefix: string;
  description: string;
  status: "active" | "paused" | "archived";
  issueCounter: number;
  budgetMonthlyCents: number | null;
  requireApprovalForNewAgents: boolean;
  brandColor: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SquadMembership {
  id: string;
  userId: string;
  squadId: string;
  role: "owner" | "admin" | "viewer";
  userDisplayName: string;
  userEmail: string;
  createdAt: string;
}

export interface CreateSquadRequest {
  name: string;
  issuePrefix: string;
  description?: string;
  budgetMonthlyCents?: number;
}

export interface UpdateSquadRequest {
  name?: string;
  description?: string;
  status?: Squad["status"];
  issuePrefix?: string;
  brandColor?: string;
  budgetMonthlyCents?: number | null;
  requireApprovalForNewAgents?: boolean;
}
```

### 9.2 Agent Types

```typescript
// web/src/types/agent.ts

export type AgentRole = "captain" | "lead" | "member";

export type AgentStatus =
  | "pending_approval"
  | "active"
  | "idle"
  | "running"
  | "error"
  | "paused"
  | "terminated";

export interface Agent {
  id: string;
  squadId: string;
  name: string;
  urlKey: string;
  role: AgentRole;
  title: string;
  status: AgentStatus;
  reportsTo: string | null;
  capabilities: string;
  adapterType: string;
  adapterConfig: Record<string, unknown>;
  runtimeConfig: Record<string, unknown>;
  budgetMonthlyCents: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface CreateAgentRequest {
  squadId: string;
  name: string;
  urlKey: string;
  role: AgentRole;
  title?: string;
  reportsTo?: string;
  capabilities?: string;
  adapterType?: string;
  adapterConfig?: Record<string, unknown>;
  runtimeConfig?: Record<string, unknown>;
  budgetMonthlyCents?: number;
}

export interface UpdateAgentRequest {
  name?: string;
  urlKey?: string;
  role?: AgentRole;
  title?: string;
  reportsTo?: string | null;
  status?: AgentStatus;
  capabilities?: string;
  adapterType?: string;
  adapterConfig?: Record<string, unknown>;
  runtimeConfig?: Record<string, unknown>;
  budgetMonthlyCents?: number | null;
}

/** Agent status badge color mapping */
export const agentStatusColors: Record<AgentStatus, string> = {
  active: "bg-green-100 text-green-800",
  idle: "bg-gray-100 text-gray-800",
  running: "bg-blue-100 text-blue-800",
  error: "bg-red-100 text-red-800",
  paused: "bg-yellow-100 text-yellow-800",
  terminated: "bg-gray-300 text-gray-600",
  pending_approval: "bg-orange-100 text-orange-800",
};
```

### 9.3 Issue Types

```typescript
// web/src/types/issue.ts

export type IssueType = "task" | "conversation";
export type IssueStatus = "backlog" | "todo" | "in_progress" | "done" | "blocked" | "cancelled";
export type IssuePriority = "critical" | "high" | "medium" | "low";

/** Valid status transitions (for the status selector UI) */
export const issueStatusTransitions: Record<IssueStatus, IssueStatus[]> = {
  backlog: ["todo", "in_progress", "cancelled"],
  todo: ["in_progress", "backlog", "blocked", "cancelled"],
  in_progress: ["done", "blocked", "cancelled"],
  blocked: ["in_progress", "todo", "cancelled"],
  done: ["todo"],
  cancelled: ["todo"],
};

export interface Issue {
  id: string;
  squadId: string;
  identifier: string;        // e.g., "ARI-42"
  type: IssueType;
  title: string;
  description: string;
  status: IssueStatus;
  priority: IssuePriority;
  parentId: string | null;
  projectId: string | null;
  goalId: string | null;
  assigneeAgentId: string | null;
  assigneeUserId: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface IssueComment {
  id: string;
  issueId: string;
  authorType: "agent" | "user" | "system";
  authorId: string;
  authorName: string;         // Resolved display name
  body: string;               // Markdown
  createdAt: string;
}

export interface CreateIssueRequest {
  squadId: string;
  title: string;
  description?: string;
  type?: IssueType;
  status?: IssueStatus;
  priority?: IssuePriority;
  parentId?: string;
  projectId?: string;
  goalId?: string;
  assigneeAgentId?: string;
  assigneeUserId?: string;
}

export interface UpdateIssueRequest {
  title?: string;
  description?: string;
  status?: IssueStatus;
  priority?: IssuePriority;
  assigneeAgentId?: string | null;
  assigneeUserId?: string | null;
  projectId?: string | null;
  goalId?: string | null;
}

export interface IssueFilters {
  status?: IssueStatus;
  priority?: IssuePriority;
  assigneeAgentId?: string;
  projectId?: string;
}
```

### 9.4 Project and Goal Types

```typescript
// web/src/types/project.ts

export type ProjectStatus = "active" | "completed" | "archived";

export interface Project {
  id: string;
  squadId: string;
  name: string;
  description: string;
  status: ProjectStatus;
  createdAt: string;
  updatedAt: string;
}

export interface CreateProjectRequest {
  name: string;
  description?: string;
}

export interface UpdateProjectRequest {
  name?: string;
  description?: string;
  status?: ProjectStatus;
}
```

```typescript
// web/src/types/goal.ts

export type GoalStatus = "active" | "completed" | "archived";

export interface Goal {
  id: string;
  squadId: string;
  parentId: string | null;
  title: string;
  description: string;
  status: GoalStatus;
  createdAt: string;
  updatedAt: string;
}

export interface CreateGoalRequest {
  title: string;
  description?: string;
  parentId?: string;
}

export interface UpdateGoalRequest {
  title?: string;
  description?: string;
  parentId?: string | null;
  status?: GoalStatus;
}
```

---

## 10. UI Components

### 10.1 shadcn/ui Components to Install

The following components are required (REQ-UI-111):

| Component | Usage |
|-----------|-------|
| `button` | All actions, form submissions |
| `input` | Text fields, search |
| `label` | Form field labels (accessibility) |
| `card` | Dashboard stats, entity summaries |
| `table` | List pages (squads, agents, issues, projects, goals) |
| `dialog` | Create/edit modals |
| `dropdown-menu` | Context menus, action menus |
| `select` | Status, priority, role selectors |
| `badge` | Status indicators, priority labels |
| `tabs` | Detail page sections (e.g., squad detail: overview/members/settings) |
| `toast` | Success/error notifications |
| `skeleton` | Loading placeholders |
| `avatar` | User and agent avatars |
| `separator` | Visual dividers |
| `sheet` | Mobile sidebar overlay |
| `command` | Quick search/command palette (future) |
| `popover` | Dropdowns, tooltips |
| `form` | Form validation with react-hook-form + zod |

Installation command:
```bash
npx shadcn@latest init
npx shadcn@latest add button input label card table dialog dropdown-menu \
  select badge tabs toast skeleton avatar separator sheet command popover form
```

### 10.2 Theme and Color Configuration

```css
/* web/src/globals.css */
@import "tailwindcss";

@layer base {
  :root {
    --background: 0 0% 100%;
    --foreground: 240 10% 3.9%;
    --card: 0 0% 100%;
    --card-foreground: 240 10% 3.9%;
    --popover: 0 0% 100%;
    --popover-foreground: 240 10% 3.9%;
    --primary: 240 5.9% 10%;
    --primary-foreground: 0 0% 98%;
    --secondary: 240 4.8% 95.9%;
    --secondary-foreground: 240 5.9% 10%;
    --muted: 240 4.8% 95.9%;
    --muted-foreground: 240 3.8% 46.1%;
    --accent: 240 4.8% 95.9%;
    --accent-foreground: 240 5.9% 10%;
    --destructive: 0 84.2% 60.2%;
    --destructive-foreground: 0 0% 98%;
    --border: 240 5.9% 90%;
    --input: 240 5.9% 90%;
    --ring: 240 5.9% 10%;
    --radius: 0.5rem;

    /* Ari-specific semantic colors */
    --status-active: 142 76% 36%;
    --status-idle: 220 9% 46%;
    --status-running: 217 91% 60%;
    --status-error: 0 84% 60%;
    --status-paused: 45 93% 47%;
    --status-terminated: 220 9% 46%;
    --status-pending: 25 95% 53%;
  }
}
```

### 10.3 Layout Components

#### AppLayout

```tsx
// web/src/components/layout/app-layout.tsx
import { Outlet } from "react-router";
import { Sidebar } from "./sidebar";
import { Header } from "./header";
import { useState } from "react";
import { Sheet, SheetContent } from "@/components/ui/sheet";

export function AppLayout() {
  const [sidebarOpen, setSidebarOpen] = useState(false);

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Desktop sidebar */}
      <aside className="hidden lg:flex lg:w-64 lg:flex-col lg:border-r">
        <Sidebar />
      </aside>

      {/* Mobile sidebar overlay */}
      <Sheet open={sidebarOpen} onOpenChange={setSidebarOpen}>
        <SheetContent side="left" className="w-64 p-0">
          <Sidebar onNavigate={() => setSidebarOpen(false)} />
        </SheetContent>
      </Sheet>

      {/* Main content */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <Header onMenuClick={() => setSidebarOpen(true)} />
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
```

#### Sidebar

```tsx
// web/src/components/layout/sidebar.tsx
import { NavLink } from "react-router";
import { useAuth } from "@/lib/auth";
import {
  LayoutDashboard,
  Users,
  Bot,
  CircleDot,
  FolderKanban,
  Target,
  LogOut,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Separator } from "@/components/ui/separator";

interface SidebarProps {
  onNavigate?: () => void;
}

const staticNavItems = [
  { to: "/", icon: LayoutDashboard, label: "Dashboard" },
  { to: "/squads", icon: Users, label: "Squads" },
];

const squadScopedNavItems = [
  { path: (id: string) => `/squads/${id}/agents`, icon: Bot, label: "Agents" },
  { path: (id: string) => `/squads/${id}/issues`, icon: CircleDot, label: "Issues" },
  { path: (id: string) => `/squads/${id}/projects`, icon: FolderKanban, label: "Projects" },
  { path: (id: string) => `/squads/${id}/goals`, icon: Target, label: "Goals" },
];

export function Sidebar({ onNavigate }: SidebarProps) {
  const { user, logout } = useAuth();
  const { activeSquad } = useActiveSquad();

  return (
    <div className="flex h-full flex-col">
      {/* Logo */}
      <div className="flex h-14 items-center px-4 font-semibold text-lg">
        Ari
      </div>
      <Separator />

      {/* Navigation */}
      <nav className="flex-1 space-y-1 px-2 py-3">
        {staticNavItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            onClick={onNavigate}
            className={({ isActive }) =>
              `flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                isActive
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
              }`
            }
          >
            <item.icon className="h-4 w-4" />
            {item.label}
          </NavLink>
        ))}
        {activeSquad && squadScopedNavItems.map((item) => {
          const to = item.path(activeSquad.id);
          return (
            <NavLink
              key={to}
              to={to}
              onClick={onNavigate}
              className={({ isActive }) =>
                `flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
                  isActive
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                }`
              }
            >
              <item.icon className="h-4 w-4" />
              {item.label}
            </NavLink>
          );
        })}
      </nav>

      {/* User section */}
      <Separator />
      <div className="flex items-center gap-3 p-4">
        <Avatar className="h-8 w-8">
          <AvatarFallback>
            {user?.displayName?.charAt(0)?.toUpperCase() ?? "?"}
          </AvatarFallback>
        </Avatar>
        <div className="flex-1 truncate text-sm">
          {user?.displayName ?? "User"}
        </div>
        <Button variant="ghost" size="icon" onClick={logout} aria-label="Log out">
          <LogOut className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
```

#### Error Boundary

```tsx
// web/src/components/layout/error-boundary.tsx
import { Component, type ErrorInfo, type ReactNode } from "react";
import { Button } from "@/components/ui/button";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("React Error Boundary caught:", error, info);
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen flex-col items-center justify-center gap-4">
          <h1 className="text-2xl font-bold">Something went wrong</h1>
          <p className="text-muted-foreground">
            {this.state.error?.message ?? "An unexpected error occurred."}
          </p>
          <Button onClick={() => this.setState({ hasError: false, error: null })}>
            Retry
          </Button>
        </div>
      );
    }

    return this.props.children;
  }
}
```

### 10.4 Agent Hierarchy Tree View

```tsx
// web/src/features/agents/agent-tree-view.tsx
import type { Agent } from "@/types/agent";
import { AgentStatusBadge } from "./agent-status-badge";

interface AgentNode {
  agent: Agent;
  children: AgentNode[];
}

function buildTree(agents: Agent[]): AgentNode[] {
  const map = new Map<string, AgentNode>();
  const roots: AgentNode[] = [];

  for (const agent of agents) {
    map.set(agent.id, { agent, children: [] });
  }

  for (const agent of agents) {
    const node = map.get(agent.id)!;
    if (agent.reportsTo && map.has(agent.reportsTo)) {
      map.get(agent.reportsTo)!.children.push(node);
    } else {
      roots.push(node);
    }
  }

  return roots;
}

function TreeNode({ node, depth }: { node: AgentNode; depth: number }) {
  return (
    <div>
      <div
        className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-accent"
        style={{ paddingLeft: `${depth * 24 + 8}px` }}
      >
        <span className="font-medium text-sm">{node.agent.name}</span>
        <span className="text-xs text-muted-foreground capitalize">
          {node.agent.role}
        </span>
        <AgentStatusBadge status={node.agent.status} />
      </div>
      {node.children.map((child) => (
        <TreeNode key={child.agent.id} node={child} depth={depth + 1} />
      ))}
    </div>
  );
}

export function AgentTreeView({ agents }: { agents: Agent[] }) {
  const tree = buildTree(agents);
  return (
    <div className="space-y-0.5">
      {tree.map((root) => (
        <TreeNode key={root.agent.id} node={root} depth={0} />
      ))}
    </div>
  );
}
```

### 10.5 Issue Status Select

```tsx
// web/src/features/issues/issue-status-select.tsx
import type { IssueStatus } from "@/types/issue";
import { issueStatusTransitions } from "@/types/issue";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const statusLabels: Record<IssueStatus, string> = {
  backlog: "Backlog",
  todo: "To Do",
  in_progress: "In Progress",
  done: "Done",
  blocked: "Blocked",
  cancelled: "Cancelled",
};

interface IssueStatusSelectProps {
  value: IssueStatus;
  onChange: (status: IssueStatus) => void;
  disabled?: boolean;
}

export function IssueStatusSelect({ value, onChange, disabled }: IssueStatusSelectProps) {
  const validTransitions = issueStatusTransitions[value];

  return (
    <Select value={value} onValueChange={(v) => onChange(v as IssueStatus)} disabled={disabled}>
      <SelectTrigger className="w-40">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {/* Current status always shown */}
        <SelectItem value={value}>{statusLabels[value]}</SelectItem>
        {/* Valid transitions */}
        {validTransitions.map((status) => (
          <SelectItem key={status} value={status}>
            {statusLabels[status]}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
```

---

## 11. Responsive Design

### 11.1 Breakpoints

| Breakpoint | Width | Behavior |
|-----------|-------|----------|
| `lg` (Desktop) | >= 1024px | Full sidebar visible, multi-column layouts |
| `md` (Tablet) | 768px - 1023px | Sidebar collapsed into Sheet overlay, single-column content |
| `sm` (Mobile) | < 768px | Stacked layout, horizontal scroll on tables |

### 11.2 Sidebar Collapse

- At >= 1024px (`lg`): sidebar is a permanent flex column on the left (width 256px).
- Below 1024px: sidebar is hidden. A hamburger icon in the header opens a `Sheet` (slide-over) containing the sidebar. The Sheet closes on navigation.

### 11.3 Table Overflow

Tables on narrow viewports use horizontal scrolling:

```tsx
<div className="overflow-x-auto">
  <Table className="min-w-[600px]">
    {/* ... */}
  </Table>
</div>
```

---

## 12. Error Handling

### 12.1 Error Layers

| Layer | Strategy |
|-------|----------|
| **Network errors** | `ApiClientError` with code `NETWORK_ERROR` triggers a toast: "Connection failed. Check your network." |
| **4xx/5xx API errors** | Parsed from `{"error": "...", "code": "..."}` and displayed via toast (REQ-UI-133) |
| **401 Unauthorized** | Global handler in `api.ts` redirects to `/login` (REQ-UI-023) |
| **React render errors** | Caught by `ErrorBoundary`, displays fallback UI with Retry button (REQ-UI-131) |
| **404 routes** | `NotFoundPage` component for unrecognized routes (REQ-UI-134) |

### 12.2 Toast Usage

```typescript
// Success: auto-dismiss after 3s
toast({ title: "Squad created successfully" });

// Error: stays until dismissed
toast({
  title: "Failed to save changes",
  description: error.message,
  variant: "destructive",
});
```

### 12.3 TanStack Query Error Handling

Mutations use `onError` callbacks to display toasts. Queries use the default retry behavior (1 retry) and display error states inline within the component:

```tsx
function SquadListPage() {
  const { data, isLoading, error } = useSquads();

  if (isLoading) return <SquadListSkeleton />;
  if (error) return <ErrorMessage message={error.message} />;

  return <SquadList squads={data!} />;
}
```

---

## 13. Loading States

### 13.1 Skeleton Placeholders

Each list and detail page defines a skeleton component that mirrors the expected layout:

```tsx
// Skeleton for a list page
function SquadListSkeleton() {
  return (
    <div className="space-y-3">
      {Array.from({ length: 5 }).map((_, i) => (
        <Skeleton key={i} className="h-16 w-full rounded-md" />
      ))}
    </div>
  );
}
```

### 13.2 Form Submission

Buttons use `disabled` + spinner pattern during mutation:

```tsx
<Button type="submit" disabled={mutation.isPending}>
  {mutation.isPending ? (
    <>
      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
      Saving...
    </>
  ) : (
    "Save"
  )}
</Button>
```

---

## 14. Performance Considerations

### 14.1 Code Splitting

All page components use `React.lazy()` with dynamic imports (see App.tsx in section 3.3). This produces separate chunks per route, keeping the initial bundle small.

### 14.2 Asset Caching

- Vite output filenames include content hashes (e.g., `main-a1b2c3d4.js`)
- The Go SPA handler sets `Cache-Control: public, max-age=31536000, immutable` for `/assets/*`
- `index.html` is served with `Cache-Control: no-cache` to ensure updates are picked up

### 14.3 Target Performance

- First Contentful Paint: < 2 seconds on broadband (REQ-UI-170)
- TanStack Query staleTime of 30s reduces redundant API calls
- Refetch on window focus keeps data fresh without polling

---

## 15. Security Considerations

### 15.1 Authentication

- Session cookies (`ari_session`) are HTTP-only, SameSite=Lax, Secure (non-localhost). The frontend never reads or stores the token; it relies on `credentials: "same-origin"` in fetch.
- The auth context only stores the user profile returned by `GET /api/auth/me`, not the token itself.

### 15.2 XSS Prevention

- React's JSX escaping prevents most XSS vectors.
- Markdown rendering (issue descriptions, comments) must use a sanitizing renderer (e.g., `react-markdown` with `rehype-sanitize`) to prevent injected scripts.
- No usage of `dangerouslySetInnerHTML`.

### 15.3 Input Validation

- Client-side validation via `react-hook-form` + `zod` provides immediate feedback.
- Server-side validation is the source of truth; client-side validation is a UX convenience only.

---

## 16. Testing Strategy

### 16.1 Unit Tests (Vitest)

**Framework:** Vitest + React Testing Library + jsdom

**Coverage Target:** >= 60% for utility functions and hooks (REQ-UI-NF-003)

**Components to Test:**

| Component | What to Test |
|-----------|-------------|
| `api.ts` | Request construction, error parsing, 401 redirect |
| `auth.tsx` | AuthProvider state transitions, useAuth hook, AuthGuard redirect logic |
| `query.ts` | Query key factory correctness |
| `agent-tree-view.tsx` | Tree building from flat agent list, rendering depth |
| `issue-status-select.tsx` | Only valid transitions shown |
| `login-page.tsx` | Form validation, error display, redirect after success |
| `sidebar.tsx` | Active nav highlighting, logout button |

**Example Test:**

```typescript
// web/tests/lib/api.test.ts
import { describe, it, expect, vi, beforeEach } from "vitest";

describe("api client", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  it("sets Content-Type for POST requests with body", async () => {
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ id: "1" }),
    });
    vi.stubGlobal("fetch", mockFetch);

    const { api } = await import("@/lib/api");
    await api.post("/squads", { name: "Test" });

    expect(mockFetch).toHaveBeenCalledWith(
      "/api/squads",
      expect.objectContaining({
        headers: expect.objectContaining({
          "Content-Type": "application/json",
        }),
      }),
    );
  });

  it("throws ApiClientError on 4xx responses", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: false,
      status: 400,
      json: () => Promise.resolve({ error: "Bad request", code: "VALIDATION_ERROR" }),
    }));

    const { api, ApiClientError } = await import("@/lib/api");

    await expect(api.get("/test")).rejects.toThrow(ApiClientError);
  });
});
```

### 16.2 End-to-End Tests (Playwright)

**Framework:** Playwright

**Workflows to Test:**

| Test | Description |
|------|-------------|
| `auth.spec.ts` | Login with valid credentials, redirect to dashboard, logout |
| `auth.spec.ts` | Login with invalid credentials, see error |
| `dashboard.spec.ts` | Dashboard loads metrics and recent activity |
| `squads.spec.ts` | Create squad, see it in list, open detail |
| `agents.spec.ts` | Create agent in squad, verify hierarchy tree |
| `issues.spec.ts` | Create issue, change status, add comment |
| `navigation.spec.ts` | Sidebar navigation, breadcrumbs, 404 page |

**Playwright Configuration:**

```typescript
// web/e2e/playwright.config.ts
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  baseURL: "http://localhost:3100",
  use: {
    headless: true,
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "make dev",
    port: 3100,
    reuseExistingServer: true,
  },
});
```

---

## 17. Package Dependencies

```json
{
  "dependencies": {
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "react-router": "^7.0.0",
    "@tanstack/react-query": "^5.0.0",
    "react-hook-form": "^7.0.0",
    "@hookform/resolvers": "^3.0.0",
    "zod": "^3.0.0",
    "react-markdown": "^9.0.0",
    "rehype-sanitize": "^6.0.0",
    "lucide-react": "^0.400.0",
    "class-variance-authority": "^0.7.0",
    "clsx": "^2.0.0",
    "tailwind-merge": "^2.0.0"
  },
  "devDependencies": {
    "typescript": "^5.5.0",
    "vite": "^6.0.0",
    "@vitejs/plugin-react": "^4.0.0",
    "tailwindcss": "^4.0.0",
    "@tailwindcss/vite": "^4.0.0",
    "vitest": "^2.0.0",
    "@testing-library/react": "^16.0.0",
    "@testing-library/jest-dom": "^6.0.0",
    "jsdom": "^25.0.0",
    "@playwright/test": "^1.45.0",
    "eslint": "^9.0.0",
    "@types/react": "^19.0.0",
    "@types/react-dom": "^19.0.0"
  }
}
```

---

## 18. Open Questions

- [x] Should the sidebar navigation for squad-scoped routes (agents, issues, etc.) always use the "active squad" context, or link generically? **Decision: Use active squad context stored in a React state/URL param.**
- [ ] Should the dashboard `GET /api/dashboard/stats` be a single aggregated endpoint or composed from multiple existing endpoints on the client side?
- [ ] Should the frontend support dark mode in Phase 1, or defer to Phase 2?
- [ ] Should Markdown rendering for issue descriptions use a full-featured editor (e.g., Milkdown) or a simple textarea with preview?

---

## 19. Alternatives Considered

### Alternative 1: Next.js SSR

**Description:** Use Next.js with server-side rendering instead of a Vite SPA.

**Pros:**
- SEO benefits (not relevant for an internal tool)
- Built-in routing and code splitting

**Cons:**
- Requires Node.js runtime, breaking the single-binary promise
- Heavier build toolchain
- More complex embedding into Go binary

**Rejected Because:** Ari's core value proposition is a single Go binary with zero external dependencies. SSR is unnecessary for an authenticated internal tool.

### Alternative 2: htmx + Go Templates

**Description:** Server-rendered HTML with htmx for dynamic updates.

**Pros:**
- No JavaScript build step
- Simpler mental model
- Smaller bundle

**Cons:**
- Limited interactivity for complex UIs (tree views, drag-and-drop, real-time)
- No established component library equivalent to shadcn/ui
- Harder to build rich forms and state machines

**Rejected Because:** The UI requirements (hierarchy tree views, status state machine selectors, real-time activity feeds) demand the interactivity that a React SPA provides.

---

## 20. Timeline Estimate

- Requirements: 1 day -- Complete
- Design: 1 day -- Complete
- Implementation: 8-10 days
  - Vite scaffold + build pipeline + Go embed: 1 day
  - Auth flow (login, context, guard): 1 day
  - Layout (sidebar, header, breadcrumbs): 1 day
  - API client + TanStack Query setup: 0.5 day
  - Dashboard page: 1 day
  - Squad pages (list + detail): 1 day
  - Agent pages (list + detail + tree): 1.5 days
  - Issue pages (list + detail + comments + status): 2 days
  - Project + Goal pages: 1 day
- Testing: 2 days
  - Unit tests (Vitest): 1 day
  - E2E tests (Playwright): 1 day
- Total: 11-13 days

---

## References

- [Requirements](./requirements.md)
- [PRD](../../core/01-PRODUCT.md)
- [User Auth Requirements](../02-user-auth/requirements.md)
- [Squad Management Requirements](../03-squad-management/requirements.md)
- [Agent Management Requirements](../04-agent-management/requirements.md)
- [Issue Tracking Requirements](../05-issue-tracking/requirements.md)
- [Projects & Goals Requirements](../06-projects-goals/requirements.md)
- [React Router v7 Docs](https://reactrouter.com/)
- [TanStack Query Docs](https://tanstack.com/query)
- [shadcn/ui Docs](https://ui.shadcn.com/)
