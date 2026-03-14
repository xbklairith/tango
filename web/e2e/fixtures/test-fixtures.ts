import { test as base, type Page, type APIRequestContext } from "@playwright/test";
import { readFileSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";

const STATE_FILE = join(tmpdir(), "ari-e2e-state.json");

interface E2EState {
  pid: number;
  baseURL: string;
  dataDir: string;
  adminEmail: string;
  adminPassword: string;
  adminName: string;
}

function loadState(): E2EState {
  return JSON.parse(readFileSync(STATE_FILE, "utf-8"));
}

interface TestFixtures {
  apiContext: APIRequestContext;
  state: E2EState;
  loginAsAdmin: (page: Page) => Promise<void>;
  seedSquad: (
    cookies: string,
    name: string,
    prefix: string,
  ) => Promise<{ id: string; name: string }>;
  seedAgent: (
    cookies: string,
    squadId: string,
    name: string,
    shortName: string,
    role: string,
    parentId?: string,
  ) => Promise<{ id: string }>;
}

export const test = base.extend<TestFixtures>({
  state: async ({}, use) => {
    await use(loadState());
  },

  apiContext: async ({ playwright }, use) => {
    const port = process.env.E2E_PORT || "3199";
    const context = await playwright.request.newContext({
      baseURL: `http://127.0.0.1:${port}`,
    });
    await use(context);
    await context.dispose();
  },

  loginAsAdmin: async ({ state }, use) => {
    const fn = async (page: Page) => {
      await page.goto("/login");
      await page.getByLabel("Email").fill(state.adminEmail);
      await page.getByLabel("Password").fill(state.adminPassword);
      await page.getByRole("button", { name: "Sign in" }).click();
      // Wait for navigation away from login
      await page.waitForURL((url) => !url.pathname.includes("/login"), {
        timeout: 10000,
      });
    };
    await use(fn);
  },

  seedSquad: async ({ apiContext, state }, use) => {
    const fn = async (cookies: string, name: string, prefix: string) => {
      const resp = await apiContext.post("/api/squads", {
        data: { name, issuePrefix: prefix },
        headers: { Cookie: cookies },
      });
      return resp.json();
    };
    await use(fn);
  },

  seedAgent: async ({ apiContext }, use) => {
    const fn = async (
      cookies: string,
      squadId: string,
      name: string,
      shortName: string,
      role: string,
      parentId?: string,
    ) => {
      const data: Record<string, string> = {
        squadId,
        name,
        shortName,
        role,
      };
      if (parentId) data.parentAgentId = parentId;
      const resp = await apiContext.post("/api/agents", {
        data,
        headers: { Cookie: cookies },
      });
      return resp.json();
    };
    await use(fn);
  },
});

export { expect } from "@playwright/test";

/**
 * Login via API and return cookies string for seeding data.
 */
export async function loginViaAPI(
  apiContext: APIRequestContext,
  email: string,
  password: string,
): Promise<string> {
  const resp = await apiContext.post("/api/auth/login", {
    data: { email, password },
  });
  const headers = resp.headersArray();
  const cookies = headers
    .filter((h) => h.name.toLowerCase() === "set-cookie")
    .map((h) => h.value.split(";")[0])
    .join("; ");
  return cookies;
}

/**
 * Register user via API.
 */
export async function registerViaAPI(
  apiContext: APIRequestContext,
  email: string,
  displayName: string,
  password: string,
): Promise<void> {
  await apiContext.post("/api/auth/register", {
    data: { email, displayName, password },
  });
}
