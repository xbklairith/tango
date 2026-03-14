import {
  createContext,
  useContext,
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
