import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";

test.describe("Journey 2: Approval Gate & Inbox Triage", () => {
  test("seed data via API, then triage inbox items via UI", async ({
    page,
    apiContext,
  }) => {
    // ── Unique user ──
    const email = `j2-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    // ── API Seeding ──
    await registerViaAPI(apiContext, email, "Marcus", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    // Create squad
    const squadResp = await apiContext.post("/api/squads", {
      data: {
        name: "Alpha Fund Research",
        issuePrefix: "AFR",
        captainName: "Commander",
        captainShortName: "commander",
      },
      headers: { Cookie: cookies },
    });
    const squad = await squadResp.json();

    // Get captain
    const agentsResp = await apiContext.get(
      `/api/agents?squadId=${squad.id}`,
      { headers: { Cookie: cookies } },
    );
    const agents = await agentsResp.json();
    const captain = agents.find(
      (a: { role: string }) => a.role === "captain",
    );

    // Create agents
    const analystResp = await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "Senior Analyst",
        shortName: "senior-analyst",
        role: "lead",
        parentAgentId: captain.id,
      },
      headers: { Cookie: cookies },
    });
    const analyst = await analystResp.json();

    const collectorResp = await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "Data Collector",
        shortName: "data-collector",
        role: "member",
        parentAgentId: analyst.id,
      },
      headers: { Cookie: cookies },
    });
    const collector = await collectorResp.json();

    // Activate agents
    await apiContext.patch(`/api/agents/${analyst.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });
    await apiContext.patch(`/api/agents/${collector.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });

    // Create goal
    await apiContext.post(`/api/squads/${squad.id}/goals`, {
      data: { title: "Q1 Research Pipeline" },
      headers: { Cookie: cookies },
    });

    // Create issues
    await apiContext.post(`/api/squads/${squad.id}/issues`, {
      data: { title: "Research NVDA earnings" },
      headers: { Cookie: cookies },
    });
    await apiContext.post(`/api/squads/${squad.id}/issues`, {
      data: { title: "Compile macro indicators" },
      headers: { Cookie: cookies },
    });

    // Create 3 inbox items
    await apiContext.post(`/api/squads/${squad.id}/inbox`, {
      data: {
        category: "approval",
        type: "trade_execution",
        title: "Approve NVDA position increase",
        body: "Senior Analyst requests to increase NVDA position by 5%. Current allocation is at 3%, proposed new allocation 8%.",
        urgency: "critical",
        relatedAgentId: analyst.id,
      },
      headers: { Cookie: cookies },
    });

    await apiContext.post(`/api/squads/${squad.id}/inbox`, {
      data: {
        category: "question",
        type: "research_scope",
        title: "Clarify macro outlook scope",
        body: "Should the macro outlook analysis include emerging markets or focus exclusively on developed markets?",
        urgency: "normal",
        relatedAgentId: collector.id,
      },
      headers: { Cookie: cookies },
    });

    await apiContext.post(`/api/squads/${squad.id}/inbox`, {
      data: {
        category: "alert",
        type: "budget_warning",
        title: "Data Collector approaching budget limit",
        body: "Data Collector has used 78% of monthly budget ($780 of $1000).",
        urgency: "normal",
        relatedAgentId: collector.id,
      },
      headers: { Cookie: cookies },
    });

    // ── Wait before UI login ──
    await new Promise((r) => setTimeout(r, 1100));

    // ══════════════════════════════════════════════
    // Act 1 — Dashboard Overview
    // ══════════════════════════════════════════════

    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    await expect(page.getByText("Alpha Fund Research")).toBeVisible({
      timeout: 10000,
    });

    // ══════════════════════════════════════════════
    // Act 2 — Inbox List & Visual Hierarchy
    // ══════════════════════════════════════════════

    await page.getByRole("navigation").getByRole("link", { name: "Inbox" }).click();

    await expect(page.getByText("Approve NVDA position increase")).toBeVisible({
      timeout: 10000,
    });
    await expect(
      page.getByText("Clarify macro outlook scope"),
    ).toBeVisible();
    await expect(
      page.getByText("Data Collector approaching budget limit"),
    ).toBeVisible();

    // Verify category badges
    await expect(page.getByText("Approval")).toBeVisible();
    await expect(page.getByText("Question")).toBeVisible();
    await expect(page.getByText("Alert").first()).toBeVisible();

    // Verify status badges: all show "Pending"
    await expect(page.getByText("Pending").first()).toBeVisible();

    // ══════════════════════════════════════════════
    // Act 3 — Filtering
    // ══════════════════════════════════════════════

    // The inbox has 3 filter comboboxes: Category, Urgency, Status
    // shadcn Select with placeholders
    const filterComboboxes = page.getByRole("combobox");

    // Filter by Category: Approval (first combobox)
    await filterComboboxes.nth(0).click();
    await page.getByRole("option", { name: "Approval" }).click();

    // Only approval item visible
    await expect(
      page.getByText("Approve NVDA position increase"),
    ).toBeVisible();
    await expect(
      page.getByText("Clarify macro outlook scope"),
    ).not.toBeVisible();

    // Clear filters
    await page.getByRole("button", { name: "Clear Filters" }).click();

    // All 3 items return
    await expect(
      page.getByText("Approve NVDA position increase"),
    ).toBeVisible({ timeout: 10000 });
    await expect(
      page.getByText("Clarify macro outlook scope"),
    ).toBeVisible();
    await expect(
      page.getByText("Data Collector approaching budget limit"),
    ).toBeVisible();

    // Filter by Urgency: Critical (second combobox)
    await filterComboboxes.nth(1).click();
    await page.getByRole("option", { name: "Critical" }).click();

    // Only critical approval item visible
    await expect(
      page.getByText("Approve NVDA position increase"),
    ).toBeVisible();
    await expect(
      page.getByText("Clarify macro outlook scope"),
    ).not.toBeVisible();

    // Clear filters
    await page.getByRole("button", { name: "Clear Filters" }).click();

    // ══════════════════════════════════════════════
    // Act 4 — Acknowledge & Resolve the Approval
    // ══════════════════════════════════════════════

    // Click the Eye icon (Acknowledge) on the approval row
    const approvalRow = page.locator("tr").filter({ hasText: "Approve NVDA position increase" });
    await approvalRow.getByRole("button", { name: "Acknowledge" }).click();

    // Status changes to "Acknowledged"
    await expect(approvalRow.getByText("Acknowledged")).toBeVisible({
      timeout: 5000,
    });

    // Click on title link to navigate to detail page
    await page.getByRole("link", { name: "Approve NVDA position increase" }).click();

    // See Category, Urgency, Status on detail page
    await expect(page.getByText("Approval")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Critical")).toBeVisible();
    await expect(page.getByText("Acknowledged")).toBeVisible();

    // See body text about NVDA
    await expect(
      page.getByText(/increase NVDA position by 5%/),
    ).toBeVisible();

    // See resolve buttons
    await expect(
      page.getByRole("button", { name: "Approve" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Reject" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Request Revision" }),
    ).toBeVisible();

    // Click "Approve"
    await page.getByRole("button", { name: "Approve" }).click();

    // Fill response note
    await page.locator("#response-note").fill(
      "Approved. Keep position under 8% of portfolio.",
    );

    // Submit resolution
    await page.getByRole("button", { name: "Submit Resolution" }).click();

    // See resolved state
    await expect(page.getByText("Resolved").first()).toBeVisible({ timeout: 5000 });

    // ══════════════════════════════════════════════
    // Act 5 — Answer the Question
    // ══════════════════════════════════════════════

    await page.getByRole("navigation").getByRole("link", { name: "Inbox" }).click();

    await page
      .getByRole("link", { name: "Clarify macro outlook scope" })
      .click();

    await expect(
      page.getByRole("button", { name: "Answered" }),
    ).toBeVisible({ timeout: 10000 });
    await expect(
      page.getByRole("button", { name: "Dismissed" }),
    ).toBeVisible();

    await page.getByRole("button", { name: "Answered" }).click();

    await page.locator("#response-note").fill(
      "Yes, include emerging markets. Focus on BRICS nations.",
    );

    await page.getByRole("button", { name: "Submit Resolution" }).click();

    await expect(page.getByText("Resolved").first()).toBeVisible({ timeout: 5000 });

    // ══════════════════════════════════════════════
    // Act 6 — Dismiss the Alert
    // ══════════════════════════════════════════════

    await page.getByRole("navigation").getByRole("link", { name: "Inbox" }).click();

    // Find the alert row and click Dismiss (X icon button)
    const alertRow = page.locator("tr").filter({ hasText: "Data Collector approaching budget limit" });
    await alertRow.getByRole("button", { name: "Dismiss" }).click();

    // Item status changes to "Resolved"
    await expect(alertRow.getByText("Resolved")).toBeVisible({
      timeout: 5000,
    });

    // ══════════════════════════════════════════════
    // Act 7 — Verify All Resolved
    // ══════════════════════════════════════════════

    // Filter by Status: Resolved (third combobox)
    const filterComboboxes2 = page.getByRole("combobox");
    await filterComboboxes2.nth(2).click();
    await page.getByRole("option", { name: "Resolved" }).click();

    // All 3 items show with "Resolved" status
    await expect(
      page.getByText("Approve NVDA position increase"),
    ).toBeVisible({ timeout: 10000 });
    await expect(
      page.getByText("Clarify macro outlook scope"),
    ).toBeVisible();
    await expect(
      page.getByText("Data Collector approaching budget limit"),
    ).toBeVisible();
  });
});
