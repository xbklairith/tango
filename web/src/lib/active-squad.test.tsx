import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type ReactNode } from "react";
import { ActiveSquadProvider, useActiveSquad } from "./active-squad";

// Mock useAuth to provide controllable user data
const mockUser = {
  id: "user-1",
  email: "test@example.com",
  displayName: "Test User",
  status: "active" as const,
  squads: [
    { squadId: "squad-1", squadName: "Alpha Squad", role: "owner" as const },
    { squadId: "squad-2", squadName: "Beta Squad", role: "admin" as const },
  ],
};

vi.mock("./auth", () => ({
  useAuth: () => ({
    user: mockUser,
    isLoading: false,
    logout: vi.fn(),
  }),
}));

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <ActiveSquadProvider>{children}</ActiveSquadProvider>
      </QueryClientProvider>
    );
  };
}

describe("ActiveSquadProvider", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("falls back to user.squads[0] when localStorage is empty", () => {
    const { result } = renderHook(() => useActiveSquad(), {
      wrapper: createWrapper(),
    });
    expect(result.current.activeSquadId).toBe("squad-1");
  });

  it("reads stored squad ID from localStorage on mount if valid", () => {
    localStorage.setItem("ari:activeSquadId", "squad-2");
    const { result } = renderHook(() => useActiveSquad(), {
      wrapper: createWrapper(),
    });
    expect(result.current.activeSquadId).toBe("squad-2");
  });

  it("falls back to squads[0] when stored ID is not in user.squads", () => {
    localStorage.setItem("ari:activeSquadId", "nonexistent-squad");
    const { result } = renderHook(() => useActiveSquad(), {
      wrapper: createWrapper(),
    });
    expect(result.current.activeSquadId).toBe("squad-1");
  });

  it("setActiveSquadId writes to localStorage", () => {
    const { result } = renderHook(() => useActiveSquad(), {
      wrapper: createWrapper(),
    });

    act(() => {
      result.current.setActiveSquadId("squad-2");
    });

    expect(localStorage.getItem("ari:activeSquadId")).toBe("squad-2");
    expect(result.current.activeSquadId).toBe("squad-2");
  });
});

describe("useActiveSquad outside provider", () => {
  it("throws when called outside ActiveSquadProvider", () => {
    expect(() => {
      renderHook(() => useActiveSquad());
    }).toThrow();
  });
});
