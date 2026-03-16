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

import { useCreateSquad } from "./use-create-squad";

describe("useCreateSquad", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls the correct API endpoint with the request data", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "squad-1", name: "Alpha" });

    const { result } = renderHook(() => useCreateSquad(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ name: "Alpha", issuePrefix: "ALP", captainName: "Captain", captainShortName: "captain" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockApiPost).toHaveBeenCalledWith("/squads", {
      name: "Alpha",
      issuePrefix: "ALP",
      captainName: "Captain",
      captainShortName: "captain",
    });
  });

  it("calls toast with 'Squad created' on success", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "squad-1", name: "Alpha" });

    const { result } = renderHook(() => useCreateSquad(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ name: "Alpha", issuePrefix: "ALP", captainName: "Captain", captainShortName: "captain" });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({ title: "Squad created" });
  });

  it("calls toast with variant 'destructive' on error", async () => {
    mockApiPost.mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useCreateSquad(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ name: "Alpha", issuePrefix: "ALP", captainName: "Captain", captainShortName: "captain" });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ variant: "destructive" }),
    );
  });

  it("uses ApiClientError message in the destructive toast", async () => {
    const { ApiClientError } = await import("@/lib/api");
    mockApiPost.mockRejectedValueOnce(
      new ApiClientError(422, "VALIDATION_ERROR", "Name is required"),
    );

    const { result } = renderHook(() => useCreateSquad(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ name: "", issuePrefix: "ALP", captainName: "Captain", captainShortName: "captain" });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({
      title: "Name is required",
      variant: "destructive",
    });
  });
});
