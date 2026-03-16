import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { type ReactNode } from "react";

const mockToast = vi.fn();
vi.mock("@/hooks/use-toast", () => ({
  useToast: () => ({ toast: mockToast }),
}));

const mockApiPost = vi.fn();
vi.mock("@/lib/api", () => ({
  api: { post: (...args: unknown[]) => mockApiPost(...args) },
  ApiClientError: class extends Error {
    status: number;
    code: string;
    constructor(s: number, c: string, m: string) {
      super(m);
      this.status = s;
      this.code = c;
      this.name = "ApiClientError";
    }
  },
}));

function createWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

import { useCreateAgent } from "./use-create-agent";

const agentPayload = {
  name: "Agent 007",
  shortName: "agent-007",
  role: "member" as const,
};

describe("useCreateAgent", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls the correct API endpoint with squadId merged into body", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "agent-1", ...agentPayload });

    const { result } = renderHook(() => useCreateAgent(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: agentPayload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockApiPost).toHaveBeenCalledWith("/agents", {
      ...agentPayload,
      squadId: "squad-1",
    });
  });

  it("invalidates agents list for the correct squadId on success", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "agent-1", ...agentPayload });

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useCreateAgent(), { wrapper });

    result.current.mutate({ squadId: "squad-1", data: agentPayload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["agents", { squadId: "squad-1" }],
      }),
    );
  });

  it("calls toast with 'Agent created' on success", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "agent-1", ...agentPayload });

    const { result } = renderHook(() => useCreateAgent(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: agentPayload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({ title: "Agent created" });
  });

  it("calls toast with variant 'destructive' on error", async () => {
    mockApiPost.mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useCreateAgent(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: agentPayload });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ variant: "destructive" }),
    );
  });

  it("uses ApiClientError message in the destructive toast", async () => {
    const { ApiClientError } = await import("@/lib/api");
    mockApiPost.mockRejectedValueOnce(
      new ApiClientError(409, "CONFLICT", "URL key already taken"),
    );

    const { result } = renderHook(() => useCreateAgent(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: agentPayload });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({
      title: "URL key already taken",
      variant: "destructive",
    });
  });
});
