import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// We re-import the module fresh per test so module-level state (refreshPromise) is reset.
async function freshApi() {
  vi.resetModules();
  const mod = await import("./api");
  return mod.api;
}

describe("api — token refresh", () => {
  const originalLocation = window.location;

  beforeEach(() => {
    vi.resetAllMocks();
    vi.resetModules();
    // jsdom doesn't let you assign window.location directly; mock it.
    Object.defineProperty(window, "location", {
      configurable: true,
      value: { href: "/", pathname: "/" },
    });
  });

  afterEach(() => {
    Object.defineProperty(window, "location", {
      configurable: true,
      value: originalLocation,
    });
  });

  it("retries the original request once after a successful refresh", async () => {
    const api = await freshApi();
    let callCount = 0;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, opts?: RequestInit) => {
        const method = opts?.method ?? "GET";

        // First call: original request → 401
        if (url === "/api/data" && method === "GET" && callCount === 0) {
          callCount++;
          return new Response(JSON.stringify({ error: "unauth" }), {
            status: 401,
          });
        }

        // Refresh endpoint → 200
        if (url === "/api/auth/refresh" && method === "POST") {
          return new Response(JSON.stringify({ message: "refreshed" }), {
            status: 200,
          });
        }

        // Retry of original request → 200 with data
        if (url === "/api/data" && method === "GET" && callCount >= 1) {
          return new Response(JSON.stringify({ value: 42 }), { status: 200 });
        }

        return new Response(null, { status: 500 });
      }),
    );

    const result = await api.get<{ value: number }>("/data");
    expect(result).toEqual({ value: 42 });
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(3); // original + refresh + retry
  });

  it("redirects to /login when refresh fails", async () => {
    const api = await freshApi();

    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, opts?: RequestInit) => {
        const method = opts?.method ?? "GET";

        if (url === "/api/data" && method === "GET") {
          return new Response(JSON.stringify({ error: "unauth" }), {
            status: 401,
          });
        }

        if (url === "/api/auth/refresh" && method === "POST") {
          return new Response(JSON.stringify({ error: "token expired" }), {
            status: 401,
          });
        }

        return new Response(null, { status: 500 });
      }),
    );

    const err = await api.get("/data").catch((e: unknown) => e as Record<string, unknown>);
    expect(err.name).toBe("ApiClientError");
    expect(err.status).toBe(401);
    expect(window.location.href).toBe("/login");
  });

  it("does not retry auth endpoints (avoids infinite loops)", async () => {
    const api = await freshApi();

    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response(JSON.stringify({ error: "unauth" }), { status: 401 }),
      ),
    );

    const err = await api.post("/auth/login", {}).catch((e: unknown) => e as Record<string, unknown>);
    expect(err.name).toBe("ApiClientError");
    expect(err.status).toBe(401);
    // Only the original request is made — no refresh, no retry.
    expect(vi.mocked(fetch)).toHaveBeenCalledTimes(1);
  });

  it("deduplicates concurrent refresh calls (all waiters share one in-flight request)", async () => {
    const api = await freshApi();
    let refreshCalls = 0;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, opts?: RequestInit) => {
        const method = opts?.method ?? "GET";

        if (url === "/api/auth/refresh" && method === "POST") {
          refreshCalls++;
          return new Response(JSON.stringify({ message: "refreshed" }), {
            status: 200,
          });
        }

        if (method === "GET") {
          // First attempt always 401, retry always 200
          if (refreshCalls === 0) {
            return new Response(null, { status: 401 });
          }
          return new Response(JSON.stringify({ ok: true }), { status: 200 });
        }

        return new Response(null, { status: 500 });
      }),
    );

    // Fire three concurrent requests that all hit 401 simultaneously.
    await Promise.all([
      api.get("/data"),
      api.get("/data"),
      api.get("/data"),
    ]);

    expect(refreshCalls).toBe(1); // Only one refresh call, not three.
  });

  it("throws ApiClientError with NETWORK_ERROR when fetch rejects", async () => {
    const api = await freshApi();

    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("Failed to fetch");
      }),
    );

    const err = await api.get("/data").catch((e: unknown) => e as Record<string, unknown>);
    expect(err.name).toBe("ApiClientError");
    expect(err.code).toBe("NETWORK_ERROR");
  });
});
