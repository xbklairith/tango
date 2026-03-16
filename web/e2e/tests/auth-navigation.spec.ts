import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";
import { LoginPage } from "../page-objects/login.page";
import { SidebarPage } from "../page-objects/sidebar.page";

test.describe("Auth & Navigation", () => {
  test("unauthenticated user is redirected to /login", async ({ page }) => {
    await page.goto("/");
    await page.waitForURL(/\/login/, { timeout: 15000 });
    await expect(page.getByLabel("Email")).toBeVisible({ timeout: 5000 });
  });

  test("invalid credentials show error message", async ({ page }) => {
    const loginPage = new LoginPage(page);
    await page.goto("/login");
    await loginPage.login("wrong@example.com", "WrongP@ss123!");
    await expect(loginPage.errorMessage).toBeVisible({ timeout: 5000 });
  });

  test("login with valid credentials navigates to dashboard", async ({
    page,
    state,
    loginAsAdmin,
  }) => {
    await page.goto("/login");
    await loginAsAdmin(page);
    await expect(page).toHaveURL(/\/$/);
  });

  test("sidebar navigation renders all sections", async ({
    page,
    apiContext,
  }) => {
    // Use a unique user for this test
    const email = `nav-test-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    await registerViaAPI(apiContext, email, "Nav Tester", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    // Seed a squad so squad-scoped nav items appear
    await apiContext.post("/api/squads", {
      data: { name: "Nav Test Squad", issuePrefix: "NAV", captainName: "NAV Captain", captainShortName: "nav-captain" },
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

    const sidebar = new SidebarPage(page);

    // Static nav items
    await expect(sidebar.dashboardLink).toBeVisible();
    await expect(sidebar.squadsLink).toBeVisible();

    // Wait for squad-scoped items to appear
    await expect(sidebar.agentsLink).toBeVisible({ timeout: 10000 });
    await expect(sidebar.issuesLink).toBeVisible();
    await expect(sidebar.projectsLink).toBeVisible();
    await expect(sidebar.goalsLink).toBeVisible();

    // Navigate to Squads
    await sidebar.navigateTo("Squads");
    await expect(page).toHaveURL(/\/squads/);

    // Navigate to Agents
    await sidebar.navigateTo("Agents");
    await expect(page).toHaveURL(/\/agents/);

    // Navigate back to Dashboard
    await sidebar.navigateTo("Dashboard");
    await expect(page).toHaveURL(/\/$/);
  });

  test("logout clears session and redirects to /login", async ({
    page,
    apiContext,
  }) => {
    // Use a unique user for this test
    const email = `logout-test-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    await registerViaAPI(apiContext, email, "Logout Tester", password);

    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    const sidebar = new SidebarPage(page);
    await sidebar.logout();

    // Should redirect to login
    await page.waitForURL(/\/login/, { timeout: 10000 });
    await expect(page.getByLabel("Email")).toBeVisible({ timeout: 5000 });

    // Trying to access dashboard should redirect back to login
    await page.goto("/");
    await page.waitForURL(/\/login/, { timeout: 10000 });
  });
});
