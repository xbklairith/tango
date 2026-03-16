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

import { useAddComment } from "./use-add-comment";

const commentPayload = { issueId: "issue-1", body: "Great work!", authorType: "user", authorId: "user-1" };

describe("useAddComment", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("calls the correct API endpoint with issueId in the URL", async () => {
    mockApiPost.mockResolvedValueOnce({
      id: "comment-1",
      issueId: "issue-1",
      body: "Great work!",
    });

    const { result } = renderHook(() => useAddComment(), {
      wrapper: createWrapper(),
    });

    result.current.mutate(commentPayload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockApiPost).toHaveBeenCalledWith("/issues/issue-1/comments", {
      body: "Great work!",
      authorType: "user",
      authorId: "user-1",
    });
  });

  it("invalidates issue comments query key on success", async () => {
    mockApiPost.mockResolvedValueOnce({
      id: "comment-1",
      issueId: "issue-1",
      body: "Great work!",
    });

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateSpy = vi.spyOn(qc, "invalidateQueries");

    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useAddComment(), { wrapper });

    result.current.mutate(commentPayload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        queryKey: ["issues", "issue-1", "comments"],
      }),
    );
  });

  it("calls toast with 'Comment added' on success", async () => {
    mockApiPost.mockResolvedValueOnce({
      id: "comment-1",
      issueId: "issue-1",
      body: "Great work!",
    });

    const { result } = renderHook(() => useAddComment(), {
      wrapper: createWrapper(),
    });

    result.current.mutate(commentPayload);

    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({ title: "Comment added" });
  });

  it("calls toast with variant 'destructive' on error", async () => {
    mockApiPost.mockRejectedValueOnce(new Error("Server error"));

    const { result } = renderHook(() => useAddComment(), {
      wrapper: createWrapper(),
    });

    result.current.mutate(commentPayload);

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith(
      expect.objectContaining({ variant: "destructive" }),
    );
  });

  it("uses ApiClientError message in the destructive toast", async () => {
    const { ApiClientError } = await import("@/lib/api");
    mockApiPost.mockRejectedValueOnce(
      new ApiClientError(422, "VALIDATION_ERROR", "Body is required"),
    );

    const { result } = renderHook(() => useAddComment(), {
      wrapper: createWrapper(),
    });

    result.current.mutate(commentPayload);

    await waitFor(() => expect(result.current.isError).toBe(true));

    expect(mockToast).toHaveBeenCalledWith({
      title: "Body is required",
      variant: "destructive",
    });
  });
});
