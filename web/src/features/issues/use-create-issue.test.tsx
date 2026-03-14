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

import { useCreateIssue } from "./use-create-issue";

const issuePayload = { title: "Fix the bug" };

describe("useCreateIssue", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls the correct API endpoint with squadId in the URL", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "issue-1", title: "Fix the bug" });

    const { result } = renderHook(() => useCreateIssue(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: issuePayload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockApiPost).toHaveBeenCalledWith(
      "/squads/squad-1/issues",
      issuePayload,
    );
  });

  it("invalidates issues list for the correct squadId on success", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "issue-1", title: "Fix the bug" });

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useCreateIssue(), { wrapper });

    result.current.mutate({ squadId: "squad-1", data: issuePayload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["issues", { squadId: "squad-1" }],
      }),
    );
  });

  it("calls toast with 'Issue created' on success", async () => {
    mockApiPost.mockResolvedValueOnce({ id: "issue-1", title: "Fix the bug" });

    const { result } = renderHook(() => useCreateIssue(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: issuePayload });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({ title: "Issue created" });
  });

  it("calls toast with variant 'destructive' on error", async () => {
    mockApiPost.mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useCreateIssue(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: issuePayload });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ variant: "destructive" }),
    );
  });

  it("uses ApiClientError message in the destructive toast", async () => {
    const { ApiClientError } = await import("@/lib/api");
    mockApiPost.mockRejectedValueOnce(
      new ApiClientError(400, "VALIDATION_ERROR", "Title is required"),
    );

    const { result } = renderHook(() => useCreateIssue(), {
      wrapper: createWrapper(),
    });

    result.current.mutate({ squadId: "squad-1", data: issuePayload });

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({
      title: "Title is required",
      variant: "destructive",
    });
  });
});
