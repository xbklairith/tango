import type { ApiError } from "@/types/api";

const API_BASE = "/api";

export class ApiClientError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiClientError";
    this.status = status;
    this.code = code;
  }
}

// Deduplicates concurrent refresh attempts — all waiters share one in-flight call.
let refreshPromise: Promise<boolean> | null = null;

async function tryRefreshSession(): Promise<boolean> {
  if (refreshPromise) return refreshPromise;

  refreshPromise = fetch(`${API_BASE}/auth/refresh`, {
    method: "POST",
    credentials: "same-origin",
  })
    .then((res) => res.ok)
    .catch(() => false)
    .finally(() => {
      refreshPromise = null;
    });

  return refreshPromise;
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
  allowRetry = true,
): Promise<T> {
  const url = `${API_BASE}${path}`;
  const headers: Record<string, string> = {};

  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      credentials: "same-origin",
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiClientError(0, "NETWORK_ERROR", "Network connection failed");
  }

  if (response.status === 401) {
    // Attempt a silent token refresh once, then replay the original request.
    // Skip retry for auth endpoints to avoid infinite loops.
    if (allowRetry && !path.startsWith("/auth/")) {
      const refreshed = await tryRefreshSession();
      if (refreshed) {
        return request<T>(method, path, body, false);
      }
    }

    // Refresh failed or not applicable — send user to login.
    if (!window.location.pathname.startsWith("/login")) {
      window.location.href = "/login";
    }
    throw new ApiClientError(401, "UNAUTHENTICATED", "Session expired");
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const data: unknown = await response.json();

  if (!response.ok) {
    const apiError = data as ApiError;
    throw new ApiClientError(
      response.status,
      apiError.code ?? "UNKNOWN",
      apiError.error ?? "An unexpected error occurred",
    );
  }

  return data as T;
}

export const api = {
  get: <T>(path: string) => request<T>("GET", path),
  post: <T>(path: string, body?: unknown) => request<T>("POST", path, body),
  patch: <T>(path: string, body?: unknown) => request<T>("PATCH", path, body),
  delete: <T>(path: string) => request<T>("DELETE", path),
};
