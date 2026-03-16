import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAuth } from "./auth";

const ACTIVE_SQUAD_KEY = "ari:activeSquadId";

interface ActiveSquadContextValue {
  activeSquadId: string | null;
  setActiveSquadId: (id: string) => void;
}

const ActiveSquadContext = createContext<ActiveSquadContextValue | null>(null);

export function ActiveSquadProvider({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const queryClient = useQueryClient();

  const [activeSquadId, setActiveSquadIdState] = useState<string | null>(() => {
    const stored = localStorage.getItem(ACTIVE_SQUAD_KEY);
    const squads = user?.squads ?? [];
    if (stored && squads.some((s) => s.squadId === stored)) {
      return stored;
    }
    const fallback = squads[0]?.squadId ?? null;
    if (fallback) {
      localStorage.setItem(ACTIVE_SQUAD_KEY, fallback);
    }
    return fallback;
  });

  // Auto-select first squad when user squads arrive asynchronously (e.g. after login)
  // Also clears stale squad IDs and redirects to home when no squads exist
  useEffect(() => {
    // Don't act until user data has actually loaded — squads array is empty
    // during initial fetch, and redirecting then would break page refreshes.
    if (!user) return;

    const squads = user.squads ?? [];

    // If current selection is still valid, keep it
    if (activeSquadId && squads.some((s) => s.squadId === activeSquadId)) return;

    // Current selection is invalid — clear it
    if (squads.length === 0) {
      setActiveSquadIdState(null);
      localStorage.removeItem(ACTIVE_SQUAD_KEY);
      // Redirect to home if on a stale squad URL
      if (window.location.pathname.includes("/squads/")) {
        window.location.href = "/";
      }
      return;
    }

    // Select the first available squad
    const fallback = squads[0]?.squadId ?? null;
    if (fallback) {
      setActiveSquadIdState(fallback);
      localStorage.setItem(ACTIVE_SQUAD_KEY, fallback);
    }
  }, [user?.squads, activeSquadId]);

  function setActiveSquadId(id: string) {
    setActiveSquadIdState(id);
    localStorage.setItem(ACTIVE_SQUAD_KEY, id);
    queryClient.invalidateQueries({ queryKey: ["agents"], exact: false });
    queryClient.invalidateQueries({ queryKey: ["issues"], exact: false });
    queryClient.invalidateQueries({ queryKey: ["projects"], exact: false });
    queryClient.invalidateQueries({ queryKey: ["goals"], exact: false });
  }

  return (
    <ActiveSquadContext.Provider value={{ activeSquadId, setActiveSquadId }}>
      {children}
    </ActiveSquadContext.Provider>
  );
}

export function useActiveSquad(): ActiveSquadContextValue {
  const ctx = useContext(ActiveSquadContext);
  if (!ctx) {
    throw new Error("useActiveSquad must be used within ActiveSquadProvider");
  }
  return ctx;
}
