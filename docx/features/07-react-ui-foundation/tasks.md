# Tasks: React UI Foundation

**Created:** 2026-03-14
**Status:** Not Started

## Requirement Traceability

- Source requirements: [./requirements.md](./requirements.md)
- Design reference: [./design.md](./design.md)
- Requirement coverage: REQ-UI-001 through REQ-UI-172, REQ-UI-NF-001 through REQ-UI-NF-004
- Missing coverage: None planned

## Implementation Approach

The UI is built bottom-up: project scaffolding and tooling first, then the typed API client and auth integration, followed by the layout shell, routing, shared components, data fetching layer, and finally the feature pages (dashboard, squads, agents, issues, projects, goals). Each page follows a consistent pattern of types, hooks, list page, and detail page. Go embedding and Makefile targets are wired up early so the full build pipeline is validated before feature pages are built. E2E tests are added last once all pages exist.

## Progress Summary

- Total Tasks: 47
- Completed: 0/47
- In Progress: None
- Test Coverage: 0%

## Tasks (TDD: Red-Green-Refactor)

---

### Component 1: Project Setup

#### Task 1.1: Initialize Vite + React 19 + TypeScript project

**Linked Requirements:** REQ-UI-001, REQ-UI-NF-001

**RED Phase:**
- [ ] Verify that `web/` directory does not exist or is empty
- [ ] Define expected structure: `web/package.json`, `web/tsconfig.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`

**GREEN Phase:**
- [ ] Scaffold Vite project with React + TypeScript template inside `web/`
  ```bash
  npm create vite@latest web -- --template react-ts
  ```
- [ ] Update `tsconfig.json` with strict mode, path aliases (`@/` -> `./src/`), and `"noUncheckedIndexedAccess": true`
  ```json
  {
    "compilerOptions": {
      "strict": true,
      "noUncheckedIndexedAccess": true,
      "baseUrl": ".",
      "paths": { "@/*": ["./src/*"] }
    }
  }
  ```
- [ ] Create `web/src/main.tsx` entry point:
  ```tsx
  import { StrictMode } from "react";
  import { createRoot } from "react-dom/client";
  import "./globals.css";

  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <div>Ari</div>
    </StrictMode>
  );
  ```
- [ ] Verify `npm run dev` starts Vite dev server and renders "Ari" in browser

**REFACTOR Phase:**
- [ ] Remove default Vite boilerplate (App.css, assets/, etc.)
- [ ] Ensure `tsconfig.json` has no `any` escape hatches

**Acceptance Criteria:**
- [ ] `npm run dev` starts Vite dev server on port 5173
- [ ] `npm run build` produces `web/dist/` with `index.html` and hashed assets
- [ ] TypeScript strict mode is enabled with zero type errors
- [ ] Path alias `@/` resolves to `web/src/`

---

#### Task 1.2: Configure Tailwind CSS 4

**Linked Requirements:** REQ-UI-001, REQ-UI-112

**RED Phase:**
- [ ] Write a test component that uses a Tailwind class; verify it renders without styles (CSS not configured yet)

**GREEN Phase:**
- [ ] Install Tailwind CSS 4 and the Vite plugin:
  ```bash
  cd web && npm install -D tailwindcss @tailwindcss/vite
  ```
- [ ] Add Tailwind plugin to `vite.config.ts`:
  ```typescript
  import tailwindcss from "@tailwindcss/vite";
  export default defineConfig({
    plugins: [react(), tailwindcss()],
  });
  ```
- [ ] Create `web/src/globals.css` with Tailwind directives and CSS custom properties for the theme:
  ```css
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
- [ ] Import `globals.css` in `main.tsx`

**REFACTOR Phase:**
- [ ] Verify no unused CSS variables remain
- [ ] Confirm Tailwind classes apply correctly in the browser

**Acceptance Criteria:**
- [ ] Tailwind utility classes render correctly in dev and production builds
- [ ] CSS custom properties for theming are defined in `:root`
- [ ] Ari-specific semantic status colors are defined

---

#### Task 1.3: Initialize shadcn/ui and install required components

**Linked Requirements:** REQ-UI-002, REQ-UI-110, REQ-UI-111

**RED Phase:**
- [ ] Attempt to import `Button` from `@/components/ui/button`; verify it fails (component not yet installed)

**GREEN Phase:**
- [ ] Initialize shadcn/ui:
  ```bash
  cd web && npx shadcn@latest init
  ```
- [ ] Install the `cn()` utility in `web/src/lib/utils.ts`:
  ```typescript
  import { clsx, type ClassValue } from "clsx";
  import { twMerge } from "tailwind-merge";
  export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs));
  }
  ```
- [ ] Install all required shadcn/ui components:
  ```bash
  npx shadcn@latest add button input label card table dialog dropdown-menu \
    select badge tabs toast skeleton avatar separator sheet command popover form
  ```
- [ ] Verify each component imports without errors

**REFACTOR Phase:**
- [ ] Confirm `components.json` has correct path aliases and style settings
- [ ] Verify no duplicate dependency installs (`class-variance-authority`, `clsx`, `tailwind-merge`)

**Acceptance Criteria:**
- [ ] All 18 shadcn/ui components are installed under `web/src/components/ui/`
- [ ] `cn()` utility is available at `@/lib/utils`
- [ ] Components render correctly with Tailwind theme variables
- [ ] `components.json` is properly configured

---

#### Task 1.4: Configure ESLint

**Linked Requirements:** REQ-UI-NF-002

**RED Phase:**
- [ ] Run `npx eslint .` and observe missing configuration error

**GREEN Phase:**
- [ ] Install ESLint 9 with flat config and TypeScript/React plugins:
  ```bash
  cd web && npm install -D eslint @eslint/js typescript-eslint eslint-plugin-react-hooks eslint-plugin-react-refresh
  ```
- [ ] Create `web/eslint.config.js` with flat config:
  ```javascript
  import js from "@eslint/js";
  import tseslint from "typescript-eslint";
  import reactHooks from "eslint-plugin-react-hooks";
  import reactRefresh from "eslint-plugin-react-refresh";

  export default tseslint.config(
    js.configs.recommended,
    ...tseslint.configs.strict,
    {
      plugins: {
        "react-hooks": reactHooks,
        "react-refresh": reactRefresh,
      },
      rules: {
        ...reactHooks.configs.recommended.rules,
        "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
        "@typescript-eslint/no-unused-vars": ["error", { argsIgnorePattern: "^_" }],
      },
    },
    { ignores: ["dist/"] },
  );
  ```
- [ ] Add lint script to `package.json`: `"lint": "eslint ."`
- [ ] Fix any initial lint errors

**REFACTOR Phase:**
- [ ] Ensure no `any` types exist (align with REQ-UI-NF-001)
- [ ] Verify zero lint errors on full codebase

**Acceptance Criteria:**
- [ ] `npm run lint` passes with zero errors
- [ ] ESLint strict TypeScript rules are enforced
- [ ] React hooks rules are enforced
- [ ] `dist/` directory is excluded from linting

---

#### Task 1.5: Configure Vitest with React Testing Library

**Linked Requirements:** REQ-UI-NF-003

**RED Phase:**
- [ ] Create a placeholder test file `web/tests/setup.test.ts` that asserts `true === true`; verify it fails because Vitest is not configured

**GREEN Phase:**
- [ ] Install Vitest and testing libraries:
  ```bash
  cd web && npm install -D vitest @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom
  ```
- [ ] Add Vitest config to `vite.config.ts`:
  ```typescript
  /// <reference types="vitest/config" />
  export default defineConfig({
    // ... existing config
    test: {
      globals: true,
      environment: "jsdom",
      setupFiles: ["./tests/setup.ts"],
      css: true,
    },
  });
  ```
- [ ] Create `web/tests/setup.ts`:
  ```typescript
  import "@testing-library/jest-dom/vitest";
  ```
- [ ] Add test script to `package.json`: `"test": "vitest run"`, `"test:watch": "vitest"`
- [ ] Verify the placeholder test passes

**REFACTOR Phase:**
- [ ] Remove placeholder test
- [ ] Ensure path aliases (`@/`) resolve correctly in test files

**Acceptance Criteria:**
- [ ] `npm test` runs Vitest with jsdom environment
- [ ] React Testing Library and jest-dom matchers are available in tests
- [ ] Path alias `@/` works in test imports
- [ ] Tests run in under 5 seconds for an empty suite

---

### Component 2: Go Embedding

#### Task 2.1: Implement SPA handler with go:embed

**Linked Requirements:** REQ-UI-003, REQ-UI-011, REQ-UI-172

**RED Phase:**
- [ ] Write a Go test that creates an HTTP test server with the SPA handler and verifies:
  - `GET /` returns `index.html` content with `Content-Type: text/html`
  - `GET /assets/main-abc123.js` returns the JS file with `Cache-Control: public, max-age=31536000, immutable`
  - `GET /squads/123` (non-file, non-API route) falls back to `index.html` with `Cache-Control: no-cache`
  - `GET /api/anything` returns 404 (API routes are not handled by SPA)
  ```go
  func TestSPAHandler_FallbackToIndex(t *testing.T) {
      // ... setup embedded test FS
      req := httptest.NewRequest("GET", "/squads/abc", nil)
      rr := httptest.NewRecorder()
      handler.ServeHTTP(rr, req)
      assert.Equal(t, http.StatusOK, rr.Code)
      assert.Contains(t, rr.Body.String(), "<div id=\"root\">")
  }
  ```

**GREEN Phase:**
- [ ] Create `internal/server/spa.go` with `go:embed` directive and `spaHandler()` function per design section 6.1
- [ ] Implement file existence check, static asset cache headers for `/assets/*`, and `index.html` fallback
- [ ] Create a minimal `web/dist/index.html` placeholder for the embed to work during development

**REFACTOR Phase:**
- [ ] Extract cache header logic into a helper function
- [ ] Add comments explaining the SPA fallback pattern

**Acceptance Criteria:**
- [ ] Static assets under `/assets/` are served with immutable cache headers
- [ ] Non-file routes fall back to `index.html` for client-side routing
- [ ] API routes (`/api/*`) are not intercepted by the SPA handler
- [ ] `index.html` is served with `Cache-Control: no-cache`
- [ ] All Go tests pass

**Notes:**
- The `//go:embed all:web/dist` directive requires `web/dist/` to exist at build time. A placeholder `index.html` is needed for `go test` to work before the frontend is built.

---

#### Task 2.2: Integrate SPA handler into router

**Linked Requirements:** REQ-UI-003, REQ-UI-011

**RED Phase:**
- [ ] Write a Go integration test that starts the full server and verifies:
  - `GET /api/health` returns JSON (API still works)
  - `GET /` returns HTML (SPA is served)
  - `GET /squads` returns HTML (SPA fallback works)

**GREEN Phase:**
- [ ] Register `spaHandler()` as the catch-all route in `internal/server/router.go`:
  ```go
  mux.Handle("/", spaHandler())
  ```
- [ ] Ensure API routes (`/api/`) are registered before the SPA catch-all

**REFACTOR Phase:**
- [ ] Verify route ordering is correct (API prefix match takes priority)

**Acceptance Criteria:**
- [ ] API endpoints continue to work unchanged
- [ ] All non-API routes serve the SPA `index.html`
- [ ] No regressions in existing Go tests

---

#### Task 2.3: Add Makefile targets for ui-dev and ui-build

**Linked Requirements:** REQ-UI-004, REQ-UI-005

**RED Phase:**
- [ ] Verify `make ui-dev` and `make ui-build` fail (targets do not exist)

**GREEN Phase:**
- [ ] Add Makefile targets:
  ```makefile
  ui-dev:
  	cd web && npm run dev

  ui-build:
  	cd web && npm run build

  build: ui-build
  	go build -o bin/ari ./cmd/ari
  ```
- [ ] Configure Vite dev server proxy in `web/vite.config.ts`:
  ```typescript
  server: {
    port: 5173,
    proxy: {
      "/api": {
        target: "http://localhost:3100",
        changeOrigin: true,
      },
    },
  },
  ```

**REFACTOR Phase:**
- [ ] Ensure `make build` runs `ui-build` first, then Go build
- [ ] Add `web/node_modules` and `web/dist` to `.gitignore` if not already present

**Acceptance Criteria:**
- [ ] `make ui-dev` starts Vite dev server on port 5173 with API proxy to `:3100`
- [ ] `make ui-build` produces `web/dist/` with production-optimized output
- [ ] `make build` builds frontend first, then embeds it into the Go binary
- [ ] Content hashes appear in output filenames (e.g., `assets/main-[hash].js`)

---

### Component 3: API Client

#### Task 3.1: Create TypeScript domain types

**Linked Requirements:** REQ-UI-NF-001, REQ-UI-NF-004

**RED Phase:**
- [ ] Write a type-level test (compile-time) that imports types from `@/types/*` and verifies type shapes match the API contract

**GREEN Phase:**
- [ ] Create `web/src/types/api.ts` with `ApiError`, `PaginatedResponse<T>`, `PaginationParams`
- [ ] Create `web/src/types/user.ts` with `AuthUser`, `AuthSquadMembership`
- [ ] Create `web/src/types/squad.ts` with `Squad`, `SquadMembership`, `CreateSquadRequest`, `UpdateSquadRequest`
- [ ] Create `web/src/types/agent.ts` with `Agent`, `AgentRole`, `AgentStatus`, `CreateAgentRequest`, `UpdateAgentRequest`, `agentStatusColors`
- [ ] Create `web/src/types/issue.ts` with `Issue`, `IssueComment`, `IssueType`, `IssueStatus`, `IssuePriority`, `IssueFilters`, `issueStatusTransitions`, `CreateIssueRequest`, `UpdateIssueRequest`
- [ ] Create `web/src/types/project.ts` with `Project`, `ProjectStatus`, `CreateProjectRequest`, `UpdateProjectRequest`
- [ ] Create `web/src/types/goal.ts` with `Goal`, `GoalStatus`, `CreateGoalRequest`, `UpdateGoalRequest`

**REFACTOR Phase:**
- [ ] Verify no `any` types exist
- [ ] Ensure all types use strict union types (not bare strings) for enums

**Acceptance Criteria:**
- [ ] All domain types compile with strict TypeScript
- [ ] Type definitions match the Go API response shapes from features 02-06
- [ ] No `any` or `unknown` escape hatches (except `adapterConfig: Record<string, unknown>`)
- [ ] Status and role types use string literal unions
- [ ] Files follow feature-based structure under `web/src/types/`

---

#### Task 3.2: Implement typed fetch wrapper (api.ts)

**Linked Requirements:** REQ-UI-020, REQ-UI-021, REQ-UI-022, REQ-UI-023, REQ-UI-024

**RED Phase:**
- [ ] Write failing tests in `web/tests/lib/api.test.ts`:
  ```typescript
  describe("api client", () => {
    it("sends GET request to /api prefix", async () => { /* ... */ });
    it("sets Content-Type: application/json for POST with body", async () => { /* ... */ });
    it("sends credentials: same-origin on every request", async () => { /* ... */ });
    it("parses ApiError from non-OK responses", async () => { /* ... */ });
    it("throws ApiClientError with status, code, message on 4xx", async () => { /* ... */ });
    it("redirects to /login on 401 response", async () => { /* ... */ });
    it("throws NETWORK_ERROR on fetch failure", async () => { /* ... */ });
    it("returns undefined for 204 No Content", async () => { /* ... */ });
    it("does not set Content-Type for GET requests", async () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/lib/api.ts` with `ApiClientError` class and `request<T>()` function
- [ ] Implement `api.get`, `api.post`, `api.patch`, `api.delete` convenience methods
- [ ] Handle 401 globally with `window.location.href = "/login"`
- [ ] Handle network errors with `NETWORK_ERROR` code
- [ ] Handle 204 No Content by returning `undefined`
- [ ] Set `credentials: "same-origin"` on all requests
- [ ] Set `Content-Type: application/json` only when body is provided

**REFACTOR Phase:**
- [ ] Extract constants (`API_BASE`)
- [ ] Ensure error messages are user-friendly
- [ ] Verify TypeScript generics are correctly typed (no implicit `any`)

**Acceptance Criteria:**
- [ ] All 9 unit tests pass
- [ ] `api.get("/squads")` sends `GET /api/squads` with credentials
- [ ] `api.post("/squads", data)` sends JSON body with correct Content-Type
- [ ] 401 responses trigger redirect to `/login`
- [ ] 4xx/5xx responses throw `ApiClientError` with parsed error code and message
- [ ] Network failures throw `ApiClientError` with code `NETWORK_ERROR`

---

### Component 4: Auth Integration

#### Task 4.1: Implement AuthProvider context and useAuth hook

**Linked Requirements:** REQ-UI-033, REQ-UI-034, REQ-UI-035

**RED Phase:**
- [ ] Write failing tests in `web/tests/lib/auth.test.tsx`:
  ```typescript
  describe("AuthProvider", () => {
    it("fetches user profile on mount via GET /api/auth/me", () => { /* ... */ });
    it("exposes user data through useAuth hook", () => { /* ... */ });
    it("sets user to null when /auth/me returns error", () => { /* ... */ });
    it("isLoading is true while fetching", () => { /* ... */ });
    it("logout calls POST /api/auth/logout and clears cache", () => { /* ... */ });
  });
  describe("useAuth", () => {
    it("throws if used outside AuthProvider", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/lib/auth.tsx` with `AuthContext`, `AuthProvider`, and `useAuth` hook
- [ ] Use TanStack Query's `useQuery` with `queryKeys.auth.me` to fetch the current user
- [ ] Implement `logout()` that calls `POST /api/auth/logout`, clears query cache, and redirects to `/login`
- [ ] Set `retry: false` on the auth query (don't retry on 401)

**REFACTOR Phase:**
- [ ] Ensure the context value is memoized to prevent unnecessary re-renders
- [ ] Type the context strictly (no optional properties that should be required)

**Acceptance Criteria:**
- [ ] `useAuth()` returns `{ user, isLoading, logout }`
- [ ] `user` is `null` when not authenticated
- [ ] `isLoading` is `true` during initial session validation
- [ ] `logout()` clears all cached data and redirects to `/login`
- [ ] `useAuth()` throws when used outside `AuthProvider`

---

#### Task 4.2: Implement AuthGuard component

**Linked Requirements:** REQ-UI-032, REQ-UI-035

**RED Phase:**
- [ ] Write failing tests in `web/tests/lib/auth-guard.test.tsx`:
  ```typescript
  describe("AuthGuard", () => {
    it("shows LoadingScreen while auth is loading", () => { /* ... */ });
    it("redirects to /login when user is null", () => { /* ... */ });
    it("renders child routes (Outlet) when user is authenticated", () => { /* ... */ });
    it("preserves original location in redirect state", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create the `AuthGuard` component in `web/src/lib/auth.tsx`:
  ```tsx
  export function AuthGuard() {
    const { user, isLoading } = useAuth();
    const location = useLocation();
    if (isLoading) return <LoadingScreen />;
    if (!user) return <Navigate to="/login" state={{ from: location }} replace />;
    return <Outlet />;
  }
  ```

**REFACTOR Phase:**
- [ ] Ensure `AuthGuard` works correctly as a layout route element

**Acceptance Criteria:**
- [ ] Loading state shows `LoadingScreen` (not a flash of login page)
- [ ] Unauthenticated users are redirected to `/login`
- [ ] The original URL is preserved in location state for post-login redirect
- [ ] Authenticated users see child route content

---

#### Task 4.3: Implement Login Page

**Linked Requirements:** REQ-UI-030, REQ-UI-031, REQ-UI-160, REQ-UI-161

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/auth/login-page.test.tsx`:
  ```typescript
  describe("LoginPage", () => {
    it("renders email and password fields with labels", () => { /* ... */ });
    it("submits credentials to POST /api/auth/login", () => { /* ... */ });
    it("redirects to / on successful login", () => { /* ... */ });
    it("redirects to original URL from location state on success", () => { /* ... */ });
    it("displays error message on failed login", () => { /* ... */ });
    it("disables submit button while submitting", () => { /* ... */ });
    it("shows 'Signing in...' text while submitting", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/auth/login-page.tsx` with email/password form
- [ ] Use `api.post("/auth/login", { email, password })` for form submission
- [ ] Invalidate `queryKeys.auth.me` after successful login to refetch user profile
- [ ] Navigate to `from` location state or `/` on success
- [ ] Display error message from `ApiClientError` on failure
- [ ] Disable submit button and show spinner during submission

**REFACTOR Phase:**
- [ ] Ensure all form inputs have associated `<Label>` elements (accessibility)
- [ ] Add `autoComplete` attributes for password managers
- [ ] Center the login card vertically and horizontally

**Acceptance Criteria:**
- [ ] Email and password fields have visible labels
- [ ] Successful login redirects to dashboard (or original route)
- [ ] Failed login shows error message without clearing the form
- [ ] Submit button shows loading state and is disabled during submission
- [ ] Form is keyboard-navigable (Tab between fields, Enter to submit)

---

### Component 5: Layout

#### Task 5.1: Implement AppLayout with sidebar and main content area

**Linked Requirements:** REQ-UI-040, REQ-UI-045, REQ-UI-120, REQ-UI-121

**RED Phase:**
- [ ] Write failing tests in `web/tests/components/layout/app-layout.test.tsx`:
  ```typescript
  describe("AppLayout", () => {
    it("renders sidebar on desktop viewport (>= 1024px)", () => { /* ... */ });
    it("hides sidebar on tablet viewport (< 1024px)", () => { /* ... */ });
    it("renders main content area with Outlet", () => { /* ... */ });
    it("opens mobile sidebar sheet on hamburger click", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/components/layout/app-layout.tsx` with:
  - Desktop sidebar (`hidden lg:flex lg:w-64`) on the left
  - Mobile sidebar using shadcn `Sheet` component
  - Header bar with hamburger menu trigger (visible below `lg`)
  - Main content area with `<Outlet />` from React Router

**REFACTOR Phase:**
- [ ] Extract sidebar width as a CSS variable or constant
- [ ] Ensure no layout shift when sidebar toggles

**Acceptance Criteria:**
- [ ] Desktop (>= 1024px): sidebar is visible, 256px wide, fixed on left
- [ ] Tablet (< 1024px): sidebar is hidden; hamburger icon opens Sheet overlay
- [ ] Main content area scrolls independently of sidebar
- [ ] Child routes render inside the main content area via `<Outlet />`

---

#### Task 5.2: Implement Sidebar navigation

**Linked Requirements:** REQ-UI-041, REQ-UI-042, REQ-UI-043, REQ-UI-160, REQ-UI-162

**RED Phase:**
- [ ] Write failing tests in `web/tests/components/layout/sidebar.test.tsx`:
  ```typescript
  describe("Sidebar", () => {
    it("renders navigation links for Dashboard, Squads, Agents, Issues, Projects, Goals", () => { /* ... */ });
    it("highlights the active navigation item", () => { /* ... */ });
    it("displays user display name", () => { /* ... */ });
    it("renders logout button with aria-label", () => { /* ... */ });
    it("calls logout on logout button click", () => { /* ... */ });
    it("calls onNavigate callback when a nav link is clicked", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/components/layout/sidebar.tsx` with:
  - Navigation items using `NavLink` from React Router with `isActive` styling
  - Lucide icons for each nav item (LayoutDashboard, Users, Bot, CircleDot, FolderKanban, Target)
  - User avatar with initials, display name, and logout button at the bottom
  - `onNavigate` callback prop for closing mobile Sheet on navigation
- [ ] Install `lucide-react`: `npm install lucide-react`

**REFACTOR Phase:**
- [ ] Extract nav items into a constant array for maintainability
- [ ] Ensure icons have proper `aria-hidden` and text labels are visible (not icon-only)

**Acceptance Criteria:**
- [ ] All 6 navigation items are rendered with icons and labels
- [ ] Active route is visually distinguished (background color change)
- [ ] User display name is shown at the bottom of the sidebar
- [ ] Logout button has `aria-label="Log out"` for accessibility
- [ ] Mobile sidebar closes on navigation (via `onNavigate` callback)

---

#### Task 5.3: Implement Header with breadcrumbs

**Linked Requirements:** REQ-UI-044, REQ-UI-045

**RED Phase:**
- [ ] Write failing tests in `web/tests/components/layout/header.test.tsx`:
  ```typescript
  describe("Header", () => {
    it("renders page title from current route", () => { /* ... */ });
    it("renders breadcrumb trail for nested routes", () => { /* ... */ });
    it("renders hamburger menu button on mobile (below lg)", () => { /* ... */ });
    it("calls onMenuClick when hamburger is clicked", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/components/layout/header.tsx` with:
  - Page title derived from the current route
  - Hamburger button (visible below `lg` breakpoint) that calls `onMenuClick`
  - Breadcrumb trail component
- [ ] Create `web/src/components/layout/breadcrumbs.tsx` with:
  - Route-to-breadcrumb mapping that resolves entity names from query cache
  - Separator between breadcrumb segments
  - Last segment is plain text (not a link)

**REFACTOR Phase:**
- [ ] Extract breadcrumb mapping into a configuration object
- [ ] Ensure breadcrumbs are keyboard-navigable

**Acceptance Criteria:**
- [ ] Header displays current page title
- [ ] Breadcrumbs show navigation path (e.g., "Dashboard > Squads > ARI Squad")
- [ ] Breadcrumb segments are clickable links (except the last one)
- [ ] Hamburger menu is only visible below 1024px
- [ ] Entity names in breadcrumbs are resolved from cache (e.g., squad name, not squad ID)

---

#### Task 5.4: Implement LoadingScreen component

**Linked Requirements:** REQ-UI-035, REQ-UI-142

**RED Phase:**
- [ ] Write a failing test that renders `LoadingScreen` and verifies a spinner/loading indicator is present with accessible attributes

**GREEN Phase:**
- [ ] Create `web/src/components/layout/loading-screen.tsx`:
  ```tsx
  import { Loader2 } from "lucide-react";
  export function LoadingScreen() {
    return (
      <div className="flex min-h-screen items-center justify-center" role="status" aria-label="Loading">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }
  ```

**REFACTOR Phase:**
- [ ] Verify `role="status"` and `aria-label="Loading"` are present for accessibility

**Acceptance Criteria:**
- [ ] Full-page centered spinner is rendered
- [ ] Screen reader accessible via `role="status"`
- [ ] Used as Suspense fallback and auth loading state

---

### Component 6: Routing

#### Task 6.1: Configure React Router v7 with route table

**Linked Requirements:** REQ-UI-010, REQ-UI-012, REQ-UI-013, REQ-UI-171

**RED Phase:**
- [ ] Write a failing test that renders the `App` component, navigates to `/squads`, and verifies the squad list page is displayed (lazy-loaded)

**GREEN Phase:**
- [ ] Install React Router v7:
  ```bash
  cd web && npm install react-router
  ```
- [ ] Create `web/src/app.tsx` with:
  - `BrowserRouter` wrapping all routes
  - `QueryClientProvider` with configured client
  - `AuthProvider` for auth context
  - `ErrorBoundary` at the top level
  - `Toaster` for toast notifications
  - Route table matching design section 5.1:
    - `/login` (public, outside AuthGuard)
    - `AuthGuard` layout route wrapping all protected routes
    - `AppLayout` layout route wrapping all authenticated routes
    - All page routes use `React.lazy()` with `<Suspense fallback={<LoadingScreen />}>`
    - `*` catch-all for 404
- [ ] Update `web/src/main.tsx` to render `<App />`

**REFACTOR Phase:**
- [ ] Verify lazy imports produce separate chunks in production build
- [ ] Ensure route transitions do not trigger full-page reloads (REQ-UI-013)

**Acceptance Criteria:**
- [ ] All 13 routes from the route table are registered:
  - `/login`, `/`, `/squads`, `/squads/:id`, `/squads/:id/agents`, `/agents/:id`, `/squads/:id/issues`, `/issues/:id`, `/squads/:id/projects`, `/projects/:id`, `/squads/:id/goals`, `/goals/:id`, `*`
- [ ] `/login` is accessible without authentication
- [ ] All other routes redirect to `/login` when unauthenticated
- [ ] Lazy loading produces separate JS chunks per page
- [ ] Route transitions are client-side only (no full page reload)
- [ ] Unrecognized routes render the 404 page

---

### Component 7: Dashboard Page

#### Task 7.1: Implement Dashboard page with stats cards

**Linked Requirements:** REQ-UI-050, REQ-UI-051, REQ-UI-054

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/dashboard/dashboard-page.test.tsx`:
  ```typescript
  describe("DashboardPage", () => {
    it("displays active agent count", () => { /* ... */ });
    it("displays total agent count", () => { /* ... */ });
    it("displays issues by status breakdown", () => { /* ... */ });
    it("displays project count", () => { /* ... */ });
    it("shows squad selector when user has multiple squads", () => { /* ... */ });
    it("shows skeleton while loading", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/dashboard/dashboard-page.tsx` as default export
- [ ] Create `web/src/features/dashboard/stats-cards.tsx` with Card components for metrics
- [ ] Create `web/src/features/dashboard/hooks.ts` with `useDashboardStats(squadId)` query hook
- [ ] Implement squad selector dropdown when user has multiple squads

**REFACTOR Phase:**
- [ ] Extract stat card into a reusable component
- [ ] Add loading skeletons that match card layout

**Acceptance Criteria:**
- [ ] Dashboard displays agent count (active / total), issue status breakdown, project count
- [ ] Squad selector appears when user belongs to multiple squads
- [ ] Loading state shows skeleton placeholders
- [ ] Error state shows inline error message

---

#### Task 7.2: Implement recent activity and quick actions

**Linked Requirements:** REQ-UI-052, REQ-UI-053

**RED Phase:**
- [ ] Write failing tests:
  ```typescript
  describe("RecentActivity", () => {
    it("renders last 10 activity items", () => { /* ... */ });
    it("shows empty state when no activity", () => { /* ... */ });
  });
  describe("QuickActions", () => {
    it("renders create agent button", () => { /* ... */ });
    it("renders create issue button", () => { /* ... */ });
    it("renders create project button", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/dashboard/recent-activity.tsx` showing last 10 activity items
- [ ] Create `web/src/features/dashboard/quick-actions.tsx` with:
  - "Create Agent" button linking to agent creation dialog/page
  - "Create Issue" button linking to issue creation dialog/page
  - "Create Project" button linking to project creation dialog/page

**REFACTOR Phase:**
- [ ] Add timestamps formatted as relative time (e.g., "2 minutes ago")
- [ ] Ensure quick action buttons use consistent sizing and icons

**Acceptance Criteria:**
- [ ] Recent activity shows up to 10 items with author, action, and timestamp
- [ ] Quick action buttons are present for agent, issue, and project creation
- [ ] Empty state message is shown when no activity exists

---

### Component 8: Squad Pages

#### Task 8.1: Implement Squad list page

**Linked Requirements:** REQ-UI-060, REQ-UI-061, REQ-UI-122, REQ-UI-140, REQ-UI-151

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/squads/squad-list-page.test.tsx`:
  ```typescript
  describe("SquadListPage", () => {
    it("renders a table of squads with name, description, status, agent count", () => { /* ... */ });
    it("shows Create Squad button", () => { /* ... */ });
    it("shows skeleton while loading", () => { /* ... */ });
    it("shows error message on fetch failure", () => { /* ... */ });
    it("navigates to squad detail on row click", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/squads/hooks.ts` with `useSquads()`, `useSquad(id)`, `useCreateSquad()`, `useUpdateSquad(id)` hooks
- [ ] Create `web/src/features/squads/squad-list-page.tsx` as default export with:
  - Table showing squad name, description, status badge, agent count
  - "Create Squad" button that opens a dialog
  - Skeleton loading state
  - Error state
- [ ] Wrap table in `overflow-x-auto` container for narrow viewports

**REFACTOR Phase:**
- [ ] Ensure table has horizontal scroll on narrow viewports (REQ-UI-122)
- [ ] Add `aria-label` to the table for accessibility

**Acceptance Criteria:**
- [ ] Squad list table renders all squad fields
- [ ] "Create Squad" button is visible and functional
- [ ] Loading shows skeleton table rows
- [ ] Clicking a row navigates to `/squads/:id`
- [ ] Table scrolls horizontally on narrow viewports

---

#### Task 8.2: Implement Squad detail page with settings

**Linked Requirements:** REQ-UI-062, REQ-UI-063, REQ-UI-064, REQ-UI-141

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/squads/squad-detail-page.test.tsx`:
  ```typescript
  describe("SquadDetailPage", () => {
    it("displays squad name, description, status, issue prefix, brand color, budget", () => { /* ... */ });
    it("allows editing squad fields via inline form", () => { /* ... */ });
    it("displays member list with roles", () => { /* ... */ });
    it("shows save button with spinner during update", () => { /* ... */ });
    it("shows success toast after save", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/squads/squad-detail-page.tsx` as default export
- [ ] Create `web/src/features/squads/squad-form.tsx` with editable fields for name, description, status, issue prefix, brand color, budget
- [ ] Create `web/src/features/squads/squad-members.tsx` showing members table with role badges
- [ ] Use `useUpdateSquad(id)` mutation with cache invalidation and toast feedback

**REFACTOR Phase:**
- [ ] Extract form validation schema with zod
- [ ] Use Tabs component for Overview / Members / Settings sections

**Acceptance Criteria:**
- [ ] All squad fields are displayed
- [ ] Fields are editable with save action
- [ ] Member list shows display name, email, and role badge (owner/admin/viewer)
- [ ] Save shows spinner on button and success toast on completion
- [ ] Error toast shown on save failure

---

### Component 9: Agent Pages

#### Task 9.1: Implement Agent list page with status badges

**Linked Requirements:** REQ-UI-070, REQ-UI-071, REQ-UI-072, REQ-UI-122, REQ-UI-162

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/agents/agent-list-page.test.tsx`:
  ```typescript
  describe("AgentListPage", () => {
    it("renders agents table with name, role, title, status, reports-to", () => { /* ... */ });
    it("renders status badges with correct colors per status", () => { /* ... */ });
    it("shows Create Agent button", () => { /* ... */ });
    it("shows skeleton while loading", () => { /* ... */ });
  });
  ```
- [ ] Write failing tests for `AgentStatusBadge`:
  ```typescript
  describe("AgentStatusBadge", () => {
    it.each(["active", "idle", "running", "error", "paused", "terminated", "pending_approval"])(
      "renders %s status with correct color and label", (status) => { /* ... */ }
    );
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/agents/hooks.ts` with `useAgents(squadId)`, `useAgent(id)`, `useCreateAgent()`, `useUpdateAgent(id)` hooks
- [ ] Create `web/src/features/agents/agent-status-badge.tsx` using shadcn Badge with color mapping from `agentStatusColors`
- [ ] Create `web/src/features/agents/agent-list-page.tsx` as default export with agents table, status badges, and create button

**REFACTOR Phase:**
- [ ] Ensure badge has both color and text label (not color-only per REQ-UI-162)
- [ ] Add horizontal scroll for table on narrow viewports

**Acceptance Criteria:**
- [ ] Agent list shows name, role, status badge, and reports-to
- [ ] Status badges use distinct colors: active=green, idle=gray, running=blue, error=red, paused=yellow, terminated=dark gray, pending_approval=orange
- [ ] Each badge includes a text label alongside the color
- [ ] "Create Agent" button opens creation dialog

---

#### Task 9.2: Implement Agent hierarchy tree view

**Linked Requirements:** REQ-UI-076

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/agents/agent-tree-view.test.tsx`:
  ```typescript
  describe("AgentTreeView", () => {
    it("builds tree from flat agent list", () => { /* ... */ });
    it("renders captain at root level", () => { /* ... */ });
    it("renders leads as children of captain", () => { /* ... */ });
    it("renders members as children of leads", () => { /* ... */ });
    it("handles agents with no parent (orphans) as root nodes", () => { /* ... */ });
    it("displays agent name, role, and status badge per node", () => { /* ... */ });
  });
  describe("buildTree", () => {
    it("returns correct tree structure from flat list", () => { /* ... */ });
    it("handles empty agent list", () => { /* ... */ });
    it("handles single agent with no children", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/agents/agent-tree-view.tsx` with:
  - `buildTree(agents: Agent[]): AgentNode[]` function that converts flat list to tree
  - `TreeNode` recursive component with indentation based on depth
  - Each node shows agent name, role label, and status badge

**REFACTOR Phase:**
- [ ] Export `buildTree` for unit testing
- [ ] Add expand/collapse toggle for tree nodes (nice to have)
- [ ] Ensure tree is keyboard-navigable

**Acceptance Criteria:**
- [ ] Flat agent list is correctly converted to hierarchical tree
- [ ] Tree renders with visual indentation per depth level
- [ ] Captain appears at root, leads as branches, members as leaves
- [ ] Each node shows name, role, and status badge
- [ ] Empty list renders nothing (no errors)

---

#### Task 9.3: Implement Agent detail page

**Linked Requirements:** REQ-UI-073, REQ-UI-074, REQ-UI-075

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/agents/agent-detail-page.test.tsx`:
  ```typescript
  describe("AgentDetailPage", () => {
    it("displays agent full profile fields", () => { /* ... */ });
    it("displays direct reports list", () => { /* ... */ });
    it("displays reporting chain (parent agents)", () => { /* ... */ });
    it("allows editing agent fields", () => { /* ... */ });
    it("shows loading skeleton while fetching", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/agents/agent-detail-page.tsx` as default export
- [ ] Create `web/src/features/agents/agent-form.tsx` with editable fields: name, role, title, status, system prompt (textarea), model, reports-to (select)
- [ ] Display hierarchy section: parent agent chain and direct reports list
- [ ] Use `useUpdateAgent(id)` mutation with cache invalidation

**REFACTOR Phase:**
- [ ] Truncate system prompt in display mode with "Show more" toggle
- [ ] Use Tabs for Profile / Hierarchy / Settings sections

**Acceptance Criteria:**
- [ ] Agent profile shows all fields: name, URL key, role, title, status, system prompt, model, reports-to, created date
- [ ] Hierarchy section shows reporting chain and direct reports
- [ ] Fields are editable with save action
- [ ] Success/error toasts on save

---

### Component 10: Issue Pages

#### Task 10.1: Implement Issue list page with filtering

**Linked Requirements:** REQ-UI-080, REQ-UI-081, REQ-UI-082, REQ-UI-122, REQ-UI-151

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/issues/issue-list-page.test.tsx`:
  ```typescript
  describe("IssueListPage", () => {
    it("renders issues table with identifier, title, status, priority, assignee, date", () => { /* ... */ });
    it("displays issue identifier in format 'PREFIX-N'", () => { /* ... */ });
    it("supports filtering by status", () => { /* ... */ });
    it("supports filtering by assignee", () => { /* ... */ });
    it("supports filtering by priority", () => { /* ... */ });
    it("supports filtering by project", () => { /* ... */ });
    it("shows Create Issue button", () => { /* ... */ });
    it("implements pagination", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/issues/hooks.ts` with `useIssues(squadId, filters)`, `useIssue(id)`, `useCreateIssue()`, `useUpdateIssue(id)`, `useIssueComments(issueId)`, `useAddComment(issueId)` hooks
- [ ] Create `web/src/features/issues/issue-filters.tsx` with filter dropdowns for status, assignee, priority, project
- [ ] Create `web/src/features/issues/issue-list-page.tsx` as default export with:
  - Table columns: identifier, title, status badge, priority badge, assignee, created date
  - Filter bar above the table
  - Pagination controls (offset/limit)
  - "Create Issue" button
- [ ] Create `web/src/hooks/use-debounce.ts` for filter input debouncing

**REFACTOR Phase:**
- [ ] Debounce filter changes to reduce API calls
- [ ] Use URL search params for filter persistence on page refresh
- [ ] Add horizontal scroll for table on narrow viewports

**Acceptance Criteria:**
- [ ] Issue list shows identifier (e.g., "ARI-42"), title, status, priority, assignee, date
- [ ] Filters for status, assignee, priority, project are functional
- [ ] Pagination controls work with offset/limit
- [ ] "Create Issue" button opens creation dialog
- [ ] Filter state is reflected in URL params

---

#### Task 10.2: Implement Issue status select with valid transitions

**Linked Requirements:** REQ-UI-087

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/issues/issue-status-select.test.tsx`:
  ```typescript
  describe("IssueStatusSelect", () => {
    it("renders current status as selected value", () => { /* ... */ });
    it("only shows valid transitions from current status", () => { /* ... */ });
    it("shows 'In Progress', 'Backlog', 'Blocked', 'Cancelled' when current is 'todo'", () => { /* ... */ });
    it("calls onChange with new status on selection", () => { /* ... */ });
    it("can be disabled", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/issues/issue-status-select.tsx` using shadcn Select component
- [ ] Use `issueStatusTransitions` map to determine valid options for the current status
- [ ] Display human-readable labels (e.g., "In Progress" instead of "in_progress")

**REFACTOR Phase:**
- [ ] Add status-specific color indicators to each option
- [ ] Ensure accessible labeling for screen readers

**Acceptance Criteria:**
- [ ] Only valid status transitions are shown in the dropdown
- [ ] Current status is always shown as selected
- [ ] Status labels are human-readable
- [ ] `onChange` fires with the new status value
- [ ] Disabled state prevents interaction

---

#### Task 10.3: Implement Issue detail page with comments thread

**Linked Requirements:** REQ-UI-083, REQ-UI-084, REQ-UI-085, REQ-UI-086, REQ-UI-141

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/issues/issue-detail-page.test.tsx`:
  ```typescript
  describe("IssueDetailPage", () => {
    it("displays full issue details including Markdown description", () => { /* ... */ });
    it("renders comments thread in chronological order", () => { /* ... */ });
    it("shows comment author (user or agent), content, timestamp", () => { /* ... */ });
    it("provides comment input form", () => { /* ... */ });
    it("submits new comment and updates thread", () => { /* ... */ });
    it("allows editing title, description, status, priority, assignee, project", () => { /* ... */ });
    it("renders issue status select with valid transitions", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/issues/issue-detail-page.tsx` as default export
- [ ] Create `web/src/features/issues/issue-form.tsx` with editable fields
- [ ] Create `web/src/features/issues/issue-comments.tsx` with:
  - Chronological comment list showing author name, content, timestamp
  - Comment input form with textarea and submit button
  - `useAddComment(issueId)` mutation that invalidates comments cache
- [ ] Install `react-markdown` and `rehype-sanitize` for Markdown rendering:
  ```bash
  npm install react-markdown rehype-sanitize
  ```
- [ ] Integrate `IssueStatusSelect` component for status changes

**REFACTOR Phase:**
- [ ] Sanitize Markdown output to prevent XSS (rehype-sanitize)
- [ ] Disable comment submit button while submitting (REQ-UI-141)
- [ ] Add optimistic update for new comments

**Acceptance Criteria:**
- [ ] Issue detail shows all fields including Markdown-rendered description
- [ ] Comments thread shows in chronological order with author, content, timestamp
- [ ] New comments can be added via the input form
- [ ] Status can be changed via the status select (valid transitions only)
- [ ] Other fields are editable inline or via edit form
- [ ] Markdown is sanitized against XSS

---

### Component 11: Project Pages

#### Task 11.1: Implement Project list page

**Linked Requirements:** REQ-UI-090, REQ-UI-091, REQ-UI-122, REQ-UI-140

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/projects/project-list-page.test.tsx`:
  ```typescript
  describe("ProjectListPage", () => {
    it("renders projects table with name, description, status, issue count", () => { /* ... */ });
    it("shows Create Project button", () => { /* ... */ });
    it("shows skeleton while loading", () => { /* ... */ });
    it("navigates to project detail on row click", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/projects/hooks.ts` with `useProjects(squadId)`, `useProject(id)`, `useCreateProject()`, `useUpdateProject(id)` hooks
- [ ] Create `web/src/features/projects/project-list-page.tsx` as default export

**REFACTOR Phase:**
- [ ] Add horizontal scroll for table on narrow viewports

**Acceptance Criteria:**
- [ ] Project list shows name, description, status, linked issue count
- [ ] "Create Project" button opens creation dialog
- [ ] Skeleton loading state is shown while fetching
- [ ] Clicking a row navigates to `/projects/:id`

---

#### Task 11.2: Implement Project detail page

**Linked Requirements:** REQ-UI-092, REQ-UI-093, REQ-UI-141

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/projects/project-detail-page.test.tsx`:
  ```typescript
  describe("ProjectDetailPage", () => {
    it("displays project name, description, status", () => { /* ... */ });
    it("displays list of linked issues", () => { /* ... */ });
    it("allows editing name, description, status", () => { /* ... */ });
    it("shows success toast after save", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/projects/project-detail-page.tsx` as default export
- [ ] Create `web/src/features/projects/project-form.tsx` with editable fields
- [ ] Display linked issues list with identifier and title

**REFACTOR Phase:**
- [ ] Extract form validation with zod

**Acceptance Criteria:**
- [ ] Project detail shows name, description, status
- [ ] Linked issues are displayed in a table
- [ ] Fields are editable with save action
- [ ] Success/error toasts on save

---

### Component 12: Goal Pages

#### Task 12.1: Implement Goal list page

**Linked Requirements:** REQ-UI-100, REQ-UI-101, REQ-UI-122, REQ-UI-140

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/goals/goal-list-page.test.tsx`:
  ```typescript
  describe("GoalListPage", () => {
    it("renders goals table with title, description, status, target date, project count", () => { /* ... */ });
    it("shows Create Goal button", () => { /* ... */ });
    it("shows skeleton while loading", () => { /* ... */ });
    it("navigates to goal detail on row click", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/goals/hooks.ts` with `useGoals(squadId)`, `useGoal(id)`, `useCreateGoal()`, `useUpdateGoal(id)` hooks
- [ ] Create `web/src/features/goals/goal-list-page.tsx` as default export

**REFACTOR Phase:**
- [ ] Add horizontal scroll for table on narrow viewports

**Acceptance Criteria:**
- [ ] Goal list shows title, description, status, target date, linked project count
- [ ] "Create Goal" button opens creation dialog
- [ ] Skeleton loading state while fetching
- [ ] Clicking a row navigates to `/goals/:id`

---

#### Task 12.2: Implement Goal detail page

**Linked Requirements:** REQ-UI-102, REQ-UI-103, REQ-UI-141

**RED Phase:**
- [ ] Write failing tests in `web/tests/features/goals/goal-detail-page.test.tsx`:
  ```typescript
  describe("GoalDetailPage", () => {
    it("displays goal title, description, status, target date", () => { /* ... */ });
    it("displays list of linked projects", () => { /* ... */ });
    it("allows editing title, description, status, target date", () => { /* ... */ });
    it("shows success toast after save", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/goals/goal-detail-page.tsx` as default export
- [ ] Create `web/src/features/goals/goal-form.tsx` with editable fields
- [ ] Display linked projects list

**REFACTOR Phase:**
- [ ] Extract form validation with zod
- [ ] Add date picker for target date field

**Acceptance Criteria:**
- [ ] Goal detail shows title, description, status, target date
- [ ] Linked projects are displayed
- [ ] Fields are editable with save action
- [ ] Success/error toasts on save

---

### Component 13: Shared Components

#### Task 13.1: Implement toast notification system

**Linked Requirements:** REQ-UI-130, REQ-UI-132, REQ-UI-133

**RED Phase:**
- [ ] Write failing tests in `web/tests/hooks/use-toast.test.ts`:
  ```typescript
  describe("useToast", () => {
    it("shows success toast with auto-dismiss", () => { /* ... */ });
    it("shows destructive toast for errors", () => { /* ... */ });
    it("displays error message from API error responses", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/hooks/use-toast.ts` (may already be provided by shadcn/ui toast installation)
- [ ] Configure Toaster component in `app.tsx`
- [ ] Success toasts auto-dismiss after 3 seconds
- [ ] Error toasts (variant: "destructive") remain until dismissed

**REFACTOR Phase:**
- [ ] Ensure toast notifications are accessible (role="alert" for errors)

**Acceptance Criteria:**
- [ ] Success toasts appear and auto-dismiss after 3 seconds
- [ ] Error toasts remain until manually dismissed
- [ ] Network errors show "Connection failed. Check your network."
- [ ] API errors show the error message from the response body
- [ ] Toasts stack vertically when multiple are shown

---

#### Task 13.2: Implement React ErrorBoundary with retry

**Linked Requirements:** REQ-UI-131

**RED Phase:**
- [ ] Write failing tests in `web/tests/components/layout/error-boundary.test.tsx`:
  ```typescript
  describe("ErrorBoundary", () => {
    it("renders children when no error", () => { /* ... */ });
    it("renders fallback UI when child throws", () => { /* ... */ });
    it("displays error message in fallback", () => { /* ... */ });
    it("renders Retry button that resets error state", () => { /* ... */ });
    it("logs error to console", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/components/layout/error-boundary.tsx` as a class component with `getDerivedStateFromError` and `componentDidCatch`
- [ ] Render fallback UI with error message and "Retry" button
- [ ] Retry button resets `hasError` state to re-attempt rendering children

**REFACTOR Phase:**
- [ ] Style fallback page to be centered and user-friendly
- [ ] Add "Go to Dashboard" link as secondary action

**Acceptance Criteria:**
- [ ] Rendering errors are caught and display fallback UI instead of blank screen
- [ ] Error message is shown to the user
- [ ] Retry button resets the boundary and re-renders children
- [ ] Errors are logged to console for debugging

---

#### Task 13.3: Implement loading skeletons

**Linked Requirements:** REQ-UI-140

**RED Phase:**
- [ ] Write a test that renders a table skeleton and verifies it uses the Skeleton component from shadcn/ui with correct row count

**GREEN Phase:**
- [ ] Create reusable skeleton patterns in `web/src/components/layout/`:
  - `TableSkeleton` (5 rows of placeholder bars approximating table layout)
  - `DetailSkeleton` (title bar + content blocks approximating detail page)
  - `CardGridSkeleton` (3 placeholder cards approximating dashboard cards)
- [ ] Use shadcn `Skeleton` component with appropriate widths and heights

**REFACTOR Phase:**
- [ ] Ensure skeletons approximate the shape of real content (per REQ-UI-140)

**Acceptance Criteria:**
- [ ] Table skeleton shows 5 rows of animated placeholder bars
- [ ] Detail skeleton shows title bar and content block placeholders
- [ ] Skeletons use the same layout dimensions as real content
- [ ] Animation is smooth and non-jarring

---

#### Task 13.4: Implement 404 NotFound page

**Linked Requirements:** REQ-UI-134

**RED Phase:**
- [ ] Write a failing test:
  ```typescript
  describe("NotFoundPage", () => {
    it("renders 404 heading", () => { /* ... */ });
    it("renders a link back to dashboard", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Create `web/src/features/not-found-page.tsx` as default export:
  ```tsx
  import { Link } from "react-router";
  import { Button } from "@/components/ui/button";

  export default function NotFoundPage() {
    return (
      <div className="flex min-h-[50vh] flex-col items-center justify-center gap-4">
        <h1 className="text-4xl font-bold">404</h1>
        <p className="text-muted-foreground">Page not found</p>
        <Button asChild>
          <Link to="/">Go to Dashboard</Link>
        </Button>
      </div>
    );
  }
  ```

**REFACTOR Phase:**
- [ ] Style consistently with error boundary fallback

**Acceptance Criteria:**
- [ ] 404 page shows "Page not found" message
- [ ] Link to dashboard is provided
- [ ] Page is centered in the main content area
- [ ] Unrecognized routes render this page

---

### Component 14: Data Fetching

#### Task 14.1: Configure TanStack Query client

**Linked Requirements:** REQ-UI-150

**RED Phase:**
- [ ] Write failing tests in `web/tests/lib/query.test.ts`:
  ```typescript
  describe("queryClient", () => {
    it("has staleTime of 30 seconds", () => { /* ... */ });
    it("has gcTime of 5 minutes", () => { /* ... */ });
    it("retries queries once", () => { /* ... */ });
    it("does not retry mutations", () => { /* ... */ });
    it("refetches on window focus", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Install TanStack Query:
  ```bash
  cd web && npm install @tanstack/react-query
  ```
- [ ] Create `web/src/lib/query.ts` with:
  ```typescript
  import { QueryClient } from "@tanstack/react-query";

  export const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        gcTime: 5 * 60_000,
        retry: 1,
        refetchOnWindowFocus: true,
      },
      mutations: {
        retry: 0,
      },
    },
  });
  ```

**REFACTOR Phase:**
- [ ] Consider adding `@tanstack/react-query-devtools` for development

**Acceptance Criteria:**
- [ ] QueryClient is configured with 30s stale time
- [ ] Queries retry once on failure
- [ ] Mutations do not retry
- [ ] Data refetches when browser tab regains focus

---

#### Task 14.2: Implement query key factory

**Linked Requirements:** REQ-UI-150, REQ-UI-152

**RED Phase:**
- [ ] Write failing tests in `web/tests/lib/query-keys.test.ts`:
  ```typescript
  describe("queryKeys", () => {
    it("squads.all returns ['squads']", () => { /* ... */ });
    it("squads.detail('1') returns ['squads', '1']", () => { /* ... */ });
    it("agents.list('sq1') returns ['agents', { squadId: 'sq1' }]", () => { /* ... */ });
    it("issues.list with filters includes filter params in key", () => { /* ... */ });
    it("auth.me returns ['auth', 'me']", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Add `queryKeys` factory to `web/src/lib/query.ts`:
  ```typescript
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
      comments: (issueId: string) => ["issues", issueId, "comments"] as const,
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

**REFACTOR Phase:**
- [ ] Ensure all hooks use `queryKeys` consistently (no hard-coded strings)

**Acceptance Criteria:**
- [ ] Query keys follow hierarchical tuple pattern
- [ ] Invalidating `["squads"]` invalidates all squad queries
- [ ] Invalidating `["squads", "1"]` invalidates only that squad's detail
- [ ] Issue list keys include filter parameters for correct cache separation
- [ ] All feature hooks reference `queryKeys` (no raw string arrays)

---

#### Task 14.3: Implement cache invalidation in mutations

**Linked Requirements:** REQ-UI-152

**RED Phase:**
- [ ] Write failing tests verifying that after a mutation completes:
  ```typescript
  describe("mutation cache invalidation", () => {
    it("useCreateSquad invalidates squads.all", () => { /* ... */ });
    it("useUpdateSquad invalidates squads.detail and squads.all", () => { /* ... */ });
    it("useCreateAgent invalidates agents.list and agents.tree", () => { /* ... */ });
    it("useAddComment invalidates issues.comments", () => { /* ... */ });
  });
  ```

**GREEN Phase:**
- [ ] Implement `onSuccess` callbacks in all mutation hooks that call `queryClient.invalidateQueries()` with the correct keys per the cache invalidation strategy table in design section 4.3:
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

**REFACTOR Phase:**
- [ ] Verify no stale data remains after mutations by checking query states in tests

**Acceptance Criteria:**
- [ ] Creating a squad invalidates the squad list cache
- [ ] Updating a squad invalidates both the list and detail caches
- [ ] Creating an agent invalidates the agent list and tree caches
- [ ] Adding a comment invalidates the comments cache for that issue
- [ ] All mutations show success/error toasts

---

### Component 15: E2E Tests

#### Task 15.1: Configure Playwright

**Linked Requirements:** REQ-UI-NF-003

**RED Phase:**
- [ ] Verify Playwright is not installed and no config exists

**GREEN Phase:**
- [ ] Install Playwright:
  ```bash
  cd web && npm install -D @playwright/test
  npx playwright install chromium
  ```
- [ ] Create `web/e2e/playwright.config.ts`:
  ```typescript
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
- [ ] Add script to `package.json`: `"test:e2e": "playwright test"`

**REFACTOR Phase:**
- [ ] Add auth helper to reuse login across tests
- [ ] Create a `web/e2e/helpers/auth.ts` with login utility

**Acceptance Criteria:**
- [ ] Playwright is configured with Chromium
- [ ] Tests target the Go dev server at `:3100`
- [ ] Screenshots are captured on failure
- [ ] `npm run test:e2e` runs the Playwright suite

---

#### Task 15.2: Write auth journey E2E test

**Linked Requirements:** REQ-UI-030, REQ-UI-031, REQ-UI-032, REQ-UI-034

**RED Phase:**
- [ ] Write failing E2E tests in `web/e2e/auth.spec.ts`:
  ```typescript
  test("login with valid credentials redirects to dashboard", async ({ page }) => {
    await page.goto("/login");
    await page.fill('input[name="email"]', "admin@example.com");
    await page.fill('input[name="password"]', "password");
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL("/");
    await expect(page.locator("text=Dashboard")).toBeVisible();
  });

  test("login with invalid credentials shows error", async ({ page }) => {
    await page.goto("/login");
    await page.fill('input[name="email"]', "wrong@example.com");
    await page.fill('input[name="password"]', "wrong");
    await page.click('button[type="submit"]');
    await expect(page.locator('[class*="destructive"]')).toBeVisible();
  });

  test("unauthenticated user is redirected to login", async ({ page }) => {
    await page.goto("/squads");
    await expect(page).toHaveURL("/login");
  });

  test("logout redirects to login", async ({ page }) => {
    // Login first, then click logout
    await page.goto("/login");
    await page.fill('input[name="email"]', "admin@example.com");
    await page.fill('input[name="password"]', "password");
    await page.click('button[type="submit"]');
    await page.click('[aria-label="Log out"]');
    await expect(page).toHaveURL("/login");
  });
  ```

**GREEN Phase:**
- [ ] Ensure the Go backend is running with seed data for tests
- [ ] Verify all auth journey tests pass

**Acceptance Criteria:**
- [ ] Valid login redirects to dashboard
- [ ] Invalid login shows error without redirect
- [ ] Unauthenticated access redirects to login
- [ ] Logout clears session and redirects to login

---

#### Task 15.3: Write navigation and 404 E2E test

**Linked Requirements:** REQ-UI-013, REQ-UI-134

**RED Phase:**
- [ ] Write failing E2E tests in `web/e2e/navigation.spec.ts`:
  ```typescript
  test("sidebar navigation works without full page reload", async ({ page }) => {
    // Login, then navigate via sidebar links
    // Verify URL changes and content updates without full reload
  });

  test("unrecognized route shows 404 page", async ({ page }) => {
    // Login, then navigate to unknown route
    await page.goto("/this-does-not-exist");
    await expect(page.locator("text=404")).toBeVisible();
    await expect(page.locator("text=Go to Dashboard")).toBeVisible();
  });

  test("breadcrumbs reflect current location", async ({ page }) => {
    // Navigate to a nested route and verify breadcrumb trail
  });
  ```

**GREEN Phase:**
- [ ] Ensure all navigation tests pass with the implemented routing

**Acceptance Criteria:**
- [ ] Sidebar links navigate without full page reload
- [ ] 404 page renders for unknown routes
- [ ] Breadcrumbs update to reflect current navigation path

---

#### Task 15.4: Write dashboard E2E test

**Linked Requirements:** REQ-UI-050, REQ-UI-051, REQ-UI-053

**RED Phase:**
- [ ] Write failing E2E test in `web/e2e/dashboard.spec.ts`:
  ```typescript
  test("dashboard displays metrics and quick actions", async ({ page }) => {
    // Login, verify dashboard loads
    await expect(page.locator("text=Active Agents")).toBeVisible();
    await expect(page.locator("text=Create Agent")).toBeVisible();
    await expect(page.locator("text=Create Issue")).toBeVisible();
    await expect(page.locator("text=Create Project")).toBeVisible();
  });
  ```

**GREEN Phase:**
- [ ] Ensure dashboard E2E test passes with seeded data

**Acceptance Criteria:**
- [ ] Dashboard loads and displays metric cards
- [ ] Quick action buttons are visible and clickable
- [ ] No console errors during dashboard rendering

---

### Integration Tasks

#### Task INT.1: Full build pipeline validation

**Objective:** Validate the complete build pipeline from `npm install` through `make build` to running the binary.

**RED Phase:**
- [ ] Run `make build` and verify it fails if frontend is not built

**GREEN Phase:**
- [ ] Run the full pipeline:
  ```bash
  cd web && npm install
  make build
  ./bin/ari run
  ```
- [ ] Verify the binary serves both API and frontend from a single process
- [ ] Verify client-side routing works on page refresh (SPA fallback)

**REFACTOR Phase:**
- [ ] Ensure `make clean` removes both `bin/` and `web/dist/`

**Acceptance Criteria:**
- [ ] `make build` produces a single binary that serves the SPA
- [ ] `http://localhost:3100/` returns the React app
- [ ] `http://localhost:3100/api/health` returns the API health check
- [ ] Direct navigation to `/squads/123` returns `index.html` (SPA fallback works)
- [ ] Static assets have content hashes in filenames
- [ ] First contentful paint is under 2 seconds (REQ-UI-170)

---

#### Task INT.2: ESLint + TypeScript + Vitest final validation

**Objective:** Ensure all automated quality checks pass with zero errors.

**RED Phase:**
- [ ] Run all checks and identify any remaining failures

**GREEN Phase:**
- [ ] Fix all issues:
  ```bash
  cd web && npm run lint && npx tsc --noEmit && npm test
  ```

**REFACTOR Phase:**
- [ ] Ensure Vitest coverage is >= 60% for utility functions and hooks (REQ-UI-NF-003)
- [ ] Ensure zero ESLint errors (REQ-UI-NF-002)
- [ ] Ensure zero TypeScript errors with strict mode (REQ-UI-NF-001)

**Acceptance Criteria:**
- [ ] `npm run lint` passes with zero errors
- [ ] `npx tsc --noEmit` passes with zero type errors
- [ ] `npm test` passes all unit tests
- [ ] Code coverage >= 60% for `lib/` and `hooks/` directories
- [ ] No `any` types in codebase (except documented exceptions)

---

### Final Verification Tasks

#### Task FV.1: Pre-Merge Checklist

**Final Checks:**

- [ ] All 47 tasks above completed
- [ ] All Vitest tests passing
- [ ] All Playwright E2E tests passing
- [ ] All Go tests passing (SPA handler)
- [ ] No linter errors (`npm run lint`)
- [ ] No type errors (`npx tsc --noEmit`)
- [ ] Test coverage >= 60% for utility functions and hooks
- [ ] No `console.log` or debug code in production paths
- [ ] No commented-out code
- [ ] Feature-based directory structure followed (REQ-UI-NF-004)
- [ ] All shadcn/ui components installed (REQ-UI-111)
- [ ] Responsive layout tested at 1024px, 768px breakpoints
- [ ] Keyboard navigation verified for all interactive elements (REQ-UI-160)
- [ ] All form inputs have labels or aria-label (REQ-UI-161)
- [ ] Color is never the sole status indicator (REQ-UI-162)
- [ ] Production build size is reasonable (< 500KB gzipped for initial load)

**Acceptance Criteria:**
- [ ] Feature is production-ready
- [ ] All quality gates passed
- [ ] Ready for PR/merge

---

## Requirement Traceability Matrix

| Requirement | Task(s) | Status |
|-------------|---------|--------|
| REQ-UI-001 | 1.1, 1.2 | Not Started |
| REQ-UI-002 | 1.3 | Not Started |
| REQ-UI-003 | 2.1, 2.2 | Not Started |
| REQ-UI-004 | 2.3 | Not Started |
| REQ-UI-005 | 2.3 | Not Started |
| REQ-UI-010 | 6.1 | Not Started |
| REQ-UI-011 | 2.1, 2.2 | Not Started |
| REQ-UI-012 | 6.1 | Not Started |
| REQ-UI-013 | 6.1, 15.3 | Not Started |
| REQ-UI-020 | 3.2 | Not Started |
| REQ-UI-021 | 3.2 | Not Started |
| REQ-UI-022 | 3.2 | Not Started |
| REQ-UI-023 | 3.2 | Not Started |
| REQ-UI-024 | 3.2 | Not Started |
| REQ-UI-030 | 4.3 | Not Started |
| REQ-UI-031 | 4.3 | Not Started |
| REQ-UI-032 | 4.2 | Not Started |
| REQ-UI-033 | 4.1 | Not Started |
| REQ-UI-034 | 4.1 | Not Started |
| REQ-UI-035 | 4.1, 4.2, 5.4 | Not Started |
| REQ-UI-040 | 5.1 | Not Started |
| REQ-UI-041 | 5.2 | Not Started |
| REQ-UI-042 | 5.2 | Not Started |
| REQ-UI-043 | 5.2 | Not Started |
| REQ-UI-044 | 5.3 | Not Started |
| REQ-UI-045 | 5.1, 5.3 | Not Started |
| REQ-UI-050 | 7.1 | Not Started |
| REQ-UI-051 | 7.1 | Not Started |
| REQ-UI-052 | 7.2 | Not Started |
| REQ-UI-053 | 7.2 | Not Started |
| REQ-UI-054 | 7.1 | Not Started |
| REQ-UI-060 | 8.1 | Not Started |
| REQ-UI-061 | 8.1 | Not Started |
| REQ-UI-062 | 8.2 | Not Started |
| REQ-UI-063 | 8.2 | Not Started |
| REQ-UI-064 | 8.2 | Not Started |
| REQ-UI-070 | 9.1 | Not Started |
| REQ-UI-071 | 9.1 | Not Started |
| REQ-UI-072 | 9.1 | Not Started |
| REQ-UI-073 | 9.3 | Not Started |
| REQ-UI-074 | 9.3 | Not Started |
| REQ-UI-075 | 9.3 | Not Started |
| REQ-UI-076 | 9.2 | Not Started |
| REQ-UI-080 | 10.1 | Not Started |
| REQ-UI-081 | 10.1 | Not Started |
| REQ-UI-082 | 10.1 | Not Started |
| REQ-UI-083 | 10.3 | Not Started |
| REQ-UI-084 | 10.3 | Not Started |
| REQ-UI-085 | 10.3 | Not Started |
| REQ-UI-086 | 10.3 | Not Started |
| REQ-UI-087 | 10.2 | Not Started |
| REQ-UI-090 | 11.1 | Not Started |
| REQ-UI-091 | 11.1 | Not Started |
| REQ-UI-092 | 11.2 | Not Started |
| REQ-UI-093 | 11.2 | Not Started |
| REQ-UI-100 | 12.1 | Not Started |
| REQ-UI-101 | 12.1 | Not Started |
| REQ-UI-102 | 12.2 | Not Started |
| REQ-UI-103 | 12.2 | Not Started |
| REQ-UI-110 | 1.3 | Not Started |
| REQ-UI-111 | 1.3 | Not Started |
| REQ-UI-112 | 1.2 | Not Started |
| REQ-UI-120 | 5.1 | Not Started |
| REQ-UI-121 | 5.1 | Not Started |
| REQ-UI-122 | 8.1, 9.1, 10.1, 11.1, 12.1 | Not Started |
| REQ-UI-130 | 13.1 | Not Started |
| REQ-UI-131 | 13.2 | Not Started |
| REQ-UI-132 | 13.1 | Not Started |
| REQ-UI-133 | 13.1 | Not Started |
| REQ-UI-134 | 13.4 | Not Started |
| REQ-UI-140 | 13.3 | Not Started |
| REQ-UI-141 | 8.2, 10.3, 11.2, 12.2 | Not Started |
| REQ-UI-142 | 5.4, 6.1 | Not Started |
| REQ-UI-150 | 14.1, 14.2 | Not Started |
| REQ-UI-151 | 8.1, 10.1 | Not Started |
| REQ-UI-152 | 14.3 | Not Started |
| REQ-UI-160 | 4.3, 5.2, FV.1 | Not Started |
| REQ-UI-161 | 4.3, FV.1 | Not Started |
| REQ-UI-162 | 9.1, 5.2, FV.1 | Not Started |
| REQ-UI-170 | INT.1 | Not Started |
| REQ-UI-171 | 6.1 | Not Started |
| REQ-UI-172 | 2.1 | Not Started |
| REQ-UI-NF-001 | 1.1, 3.1, INT.2 | Not Started |
| REQ-UI-NF-002 | 1.4, INT.2 | Not Started |
| REQ-UI-NF-003 | 1.5, 15.1, INT.2 | Not Started |
| REQ-UI-NF-004 | 3.1 | Not Started |

## Dependency Graph

```
1.1 (Vite+React) ──> 1.2 (Tailwind) ──> 1.3 (shadcn/ui) ──> 5.x (Layout)
       │                                        │
       ├──> 1.4 (ESLint)                        ├──> 4.3 (Login Page)
       ├──> 1.5 (Vitest)                        │
       │                                        └──> 13.x (Shared Components)
       ├──> 3.1 (Types) ──> 3.2 (API Client) ──> 4.1 (AuthProvider)
       │                                              │
       │                                              ├──> 4.2 (AuthGuard)
       │                                              │         │
       │                                              │         └──> 6.1 (Routing) ──> 7.x (Dashboard)
       │                                              │                                  │
       │                                              │                                  ├──> 8.x (Squads)
       │                                              │                                  ├──> 9.x (Agents)
       │                                              │                                  ├──> 10.x (Issues)
       │                                              │                                  ├──> 11.x (Projects)
       │                                              │                                  └──> 12.x (Goals)
       │                                              │
       │                                              └──> 14.x (Data Fetching)
       │
       └──> 2.x (Go Embedding) [can be parallel with frontend tasks]

15.x (E2E) depends on all feature pages being complete
INT.1 depends on 2.x + all frontend tasks
INT.2 depends on all tasks
FV.1 depends on all tasks
```

## Task Tracking Legend

- `[ ]` - Not started
- `[~]` - In progress
- `[x]` - Completed

## Commit Strategy

After each completed task:
```bash
# After RED phase
git add web/tests/
git commit -m "test: Add tests for [functionality]"

# After GREEN phase
git add web/src/
git commit -m "feat: Implement [functionality]"

# After REFACTOR phase
git add web/src/
git commit -m "refactor: Optimize [component]"
```

## Notes

### Implementation Notes

- Task 14.x (Data Fetching) tasks are placed late in the document but should be implemented early (after 3.2), since all feature hooks depend on TanStack Query and the query key factory.
- Component 13 (Shared Components) tasks can be implemented in parallel with feature pages as needed.
- Go embedding (Component 2) is independent of frontend tasks and can proceed in parallel.

### Blockers

- [ ] Open question: Should dashboard stats come from a single aggregated endpoint or multiple existing endpoints?
- [ ] Open question: Dark mode support deferred to Phase 2?
- [ ] Open question: Markdown editor vs textarea with preview?

### Future Improvements

- Dark mode support (Phase 2)
- Real-time updates via SSE for activity feed and issue comments
- Command palette (Cmd+K) using shadcn Command component
- Drag-and-drop issue status changes (kanban board view)
- Rich Markdown editor (e.g., Milkdown) for issue descriptions
- Agent runtime monitoring dashboard with live metrics

### Lessons Learned

[Document insights gained during implementation]
