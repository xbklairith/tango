import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";

test.describe("Squad Management", () => {
  test("squad list shows seeded squad", async ({
    page,
    apiContext,
  }) => {
    // Use a unique user for this test to avoid session collisions
    const email = `squad-test-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    await registerViaAPI(apiContext, email, "Squad Tester", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    const resp = await apiContext.post("/api/squads", {
      data: { name: "Squad List Test", issuePrefix: "SLT", captainName: "SLT Captain", captainShortName: "slt-captain" },
      headers: { Cookie: cookies },
    });
    expect(resp.ok()).toBeTruthy();

    // Login via UI with same user
    await new Promise((r) => setTimeout(r, 1100));
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    await page.goto("/squads");

    // Verify squad is visible
    await expect(page.getByText("Squad List Test")).toBeVisible({ timeout: 10000 });
  });
});
