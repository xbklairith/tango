import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";
import { DashboardPage } from "../page-objects/dashboard.page";

test.describe("Dashboard Stats", () => {
  test("no squad shows welcome message", async ({ page, apiContext }) => {
    // Register a fresh user with no squads
    const email = `nosquad-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    await registerViaAPI(apiContext, email, "No Squad User", password);

    // Login via UI
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    const dashboard = new DashboardPage(page);
    await expect(dashboard.welcomeMessage).toBeVisible({ timeout: 5000 });
  });

  test("with squad data shows correct stat cards", async ({
    page,
    apiContext,
  }) => {
    // Use a unique user for this test
    const email = `dashboard-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    await registerViaAPI(apiContext, email, "Dashboard Tester", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    // Create squad
    const squadResp = await apiContext.post("/api/squads", {
      data: { name: "Dashboard Squad", issuePrefix: "DASH", captainName: "Dashboard Captain", captainShortName: "dash-captain" },
      headers: { Cookie: cookies },
    });
    const squad = await squadResp.json();

    // Create issues
    await apiContext.post(`/api/squads/${squad.id}/issues`, {
      data: { title: "Backlog Issue" },
      headers: { Cookie: cookies },
    });

    const issueResp = await apiContext.post(`/api/squads/${squad.id}/issues`, {
      data: { title: "In Progress Issue" },
      headers: { Cookie: cookies },
    });
    const issue = await issueResp.json();
    await apiContext.patch(`/api/issues/${issue.id}`, {
      data: { status: "in_progress" },
      headers: { Cookie: cookies },
    });

    // Create project
    await apiContext.post(`/api/squads/${squad.id}/projects`, {
      data: { name: "Dashboard Project" },
      headers: { Cookie: cookies },
    });

    // Login via UI
    await new Promise((r) => setTimeout(r, 1100));
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    const dashboard = new DashboardPage(page);

    // Wait for squad name to appear
    await expect(dashboard.squadName).toBeVisible({ timeout: 10000 });

    // Verify stat cards exist
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "Active Agents" })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "In Progress" })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "Open Issues" })).toBeVisible();
    await expect(page.locator('[data-slot="card-title"]').filter({ hasText: "Projects" })).toBeVisible();
  });
});
