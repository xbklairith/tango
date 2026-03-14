import {
  createContext,
  useContext,
  type ReactNode,
} from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./api";
import { queryKeys } from "./query";
import type { AuthUser } from "@/types/user";
import { LoadingScreen } from "@/components/layout/loading-screen";

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
    retry: false,
    staleTime: 5 * 60_000,
  });

  async function logout() {
    await api.post("/auth/logout");
    localStorage.removeItem("ari:activeSquadId");
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
