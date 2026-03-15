import { BrowserRouter, Routes, Route } from "react-router";
import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "./lib/query";
import { AuthProvider, AuthGuard } from "./lib/auth";
import { ActiveSquadProvider } from "./lib/active-squad";
import { AppLayout } from "./components/layout/app-layout";
import { ErrorBoundary } from "./components/layout/error-boundary";
import { Toaster } from "./components/ui/toaster";
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
const ConversationListPage = lazy(() => import("./features/conversations/conversation-list-page"));
const ConversationPage = lazy(() => import("./features/conversations/conversation-page"));
const InboxListPage = lazy(() => import("./features/inbox/inbox-list-page"));
const InboxDetailPage = lazy(() => import("./features/inbox/inbox-detail-page"));
const PipelinesPage = lazy(() => import("./features/pipelines/pipelines-page"));
const PipelineDetailPage = lazy(() => import("./features/pipelines/pipeline-detail-page"));
const NotFoundPage = lazy(() => import("./features/not-found-page"));

export function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <AuthProvider>
            <ActiveSquadProvider>
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
                  <Route path="squads/:id/conversations" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <ConversationListPage />
                    </Suspense>
                  } />
                  <Route path="conversations/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <ConversationPage />
                    </Suspense>
                  } />
                  <Route path="squads/:id/inbox" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <InboxListPage />
                    </Suspense>
                  } />
                  <Route path="inbox/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <InboxDetailPage />
                    </Suspense>
                  } />
                  <Route path="pipelines" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <PipelinesPage />
                    </Suspense>
                  } />
                  <Route path="pipelines/:id" element={
                    <Suspense fallback={<LoadingScreen />}>
                      <PipelineDetailPage />
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
            </ActiveSquadProvider>
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  );
}
