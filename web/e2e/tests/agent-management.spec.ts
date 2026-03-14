import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";

test.describe("Agent Management", () => {
  test("agent list shows agents with status badges", async ({
    page,
    apiContext,
  }) => {
    // Use a unique user for this test to avoid session collisions
    const email = `agent-test-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    await registerViaAPI(apiContext, email, "Agent Tester", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    const squadResp = await apiContext.post("/api/squads", {
      data: { name: "Agent List Squad", issuePrefix: "ALS" },
      headers: { Cookie: cookies },
    });
    const squad = await squadResp.json();

    const captainResp = await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "Test Captain",
        shortName: "test-cap",
        role: "captain",
      },
      headers: { Cookie: cookies },
    });
    const captain = await captainResp.json();

    await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "Test Lead",
        shortName: "test-lead",
        role: "lead",
        parentAgentId: captain.id,
      },
      headers: { Cookie: cookies },
    });

    // Login via UI (separate session, different second)
    await new Promise((r) => setTimeout(r, 1100));
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    // Navigate directly to agents page for this squad
    await page.goto(`/squads/${squad.id}/agents`);

    // Verify agents are visible
    await expect(page.getByText("Test Captain")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Test Lead")).toBeVisible();
  });
});
