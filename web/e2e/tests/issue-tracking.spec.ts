import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";

test.describe("Issue Tracking", () => {
  test("issue list shows issues with identifiers", async ({
    page,
    apiContext,
  }) => {
    // Use a unique user for this test
    const email = `issue-test-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    await registerViaAPI(apiContext, email, "Issue Tester", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    const squadResp = await apiContext.post("/api/squads", {
      data: { name: "Issue List Squad", issuePrefix: "ILS" },
      headers: { Cookie: cookies },
    });
    const squad = await squadResp.json();

    await apiContext.post(`/api/squads/${squad.id}/issues`, {
      data: { title: "First Issue E2E" },
      headers: { Cookie: cookies },
    });

    await apiContext.post(`/api/squads/${squad.id}/issues`, {
      data: { title: "Second Issue E2E" },
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

    await page.goto(`/squads/${squad.id}/issues`);

    // Verify issues are visible with identifiers
    await expect(page.getByText("First Issue E2E")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Second Issue E2E")).toBeVisible();
    await expect(page.getByText("ILS-1")).toBeVisible();
    await expect(page.getByText("ILS-2")).toBeVisible();
  });
});
