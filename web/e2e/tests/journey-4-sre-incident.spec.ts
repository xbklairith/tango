import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";

test.describe("Journey 4: DevOps/SRE Incident Response", () => {
  test("seed SRE squad via API, then triage incidents and coordinate response via UI", async ({
    page,
    apiContext,
  }) => {
    // ── Unique user ──
    const email = `j4-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    // ── API Seeding ──

    // Register & login
    await registerViaAPI(apiContext, email, "Alex", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    // Create squad
    const squadResp = await apiContext.post("/api/squads", {
      data: {
        name: "SRE On-Call",
        issuePrefix: "SRE",
        captainName: "Incident Commander",
        captainShortName: "incident-commander",
      },
      headers: { Cookie: cookies },
    });
    const squad = await squadResp.json();

    // Get captain
    const agentsResp = await apiContext.get(
      `/api/agents?squadId=${squad.id}`,
      { headers: { Cookie: cookies } },
    );
    const agentsList = await agentsResp.json();
    const captain = agentsList.find(
      (a: { role: string }) => a.role === "captain",
    );

    // Create agents
    const triagerResp = await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "Alert Triager",
        shortName: "alert-triager",
        role: "lead",
        parentAgentId: captain.id,
      },
      headers: { Cookie: cookies },
    });
    const triager = await triagerResp.json();

    const executorResp = await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "Runbook Executor",
        shortName: "runbook-executor",
        role: "member",
        parentAgentId: triager.id,
      },
      headers: { Cookie: cookies },
    });
    const executor = await executorResp.json();

    // Activate all agents
    await apiContext.patch(`/api/agents/${captain.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });
    await apiContext.patch(`/api/agents/${triager.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });
    await apiContext.patch(`/api/agents/${executor.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });

    // Create 3 inbox items
    await apiContext.post(`/api/squads/${squad.id}/inbox`, {
      data: {
        category: "alert",
        type: "infrastructure",
        title: "Production database CPU at 95%",
        body: "Database CPU utilization has been above 95% for the last 10 minutes.",
        urgency: "critical",
        relatedAgentId: triager.id,
      },
      headers: { Cookie: cookies },
    });

    const latencyResp = await apiContext.post(`/api/squads/${squad.id}/inbox`, {
      data: {
        category: "alert",
        type: "latency",
        title: "API latency spike detected - p99 above 5s",
        body: "API p99 latency has exceeded 5 second threshold for /api/v2/orders endpoint.",
        urgency: "critical",
        relatedAgentId: triager.id,
      },
      headers: { Cookie: cookies },
    });
    expect(latencyResp.ok()).toBeTruthy();

    await apiContext.post(`/api/squads/${squad.id}/inbox`, {
      data: {
        category: "decision",
        type: "scaling",
        title: "Auto-scale cluster beyond budget limit?",
        body: "Current cluster is at 90% capacity. Auto-scaling would exceed monthly budget by approximately $2,400.",
        urgency: "critical",
        relatedAgentId: executor.id,
      },
      headers: { Cookie: cookies },
    });

    // ── Wait before UI login ──
    await new Promise((r) => setTimeout(r, 1100));

    // ══════════════════════════════════════════════
    // Act 1 — Inbox Triage
    // ══════════════════════════════════════════════

    // Login via UI
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    // Navigate to Inbox via sidebar
    await page.getByRole("navigation").getByRole("link", { name: "Inbox" }).click();

    // See 3 items, all with critical urgency
    await expect(
      page.getByText("Production database CPU at 95%"),
    ).toBeVisible({ timeout: 10000 });
    await expect(
      page.getByText("API latency spike detected"),
    ).toBeVisible();
    await expect(
      page.getByText("Auto-scale cluster beyond budget limit?"),
    ).toBeVisible();

    // Category badges: "Alert" x2, "Decision" x1
    const alertBadges = page.getByText("Alert", { exact: true });
    await expect(alertBadges.first()).toBeVisible();
    await expect(page.getByText("Decision")).toBeVisible();

    // ── Dismiss from list (fast triage) ──
    const cpuRow = page
      .locator("tr, [data-testid]")
      .filter({ hasText: "Production database CPU at 95%" });
    await cpuRow.getByRole("button", { name: "Dismiss" }).click();

    // Item status changes to "Resolved" in-place
    await expect(cpuRow.getByText("Resolved")).toBeVisible({ timeout: 5000 });

    // ── Handle second alert via detail page ──
    await page
      .getByRole("link", { name: "API latency spike detected" })
      .click();
    await page.waitForURL(/\/inbox\//, { timeout: 10000 });

    // See the "Acknowledge" button (visible because pending)
    const ackButton = page.getByRole("button", { name: "Acknowledge" }).first();
    await expect(ackButton).toBeVisible({ timeout: 10000 });

    // Click "Acknowledge" → status changes to "Acknowledged"
    await ackButton.click();
    await expect(page.getByText("Acknowledged")).toBeVisible({ timeout: 5000 });

    // For alert category, should show "Dismiss" button (not Approve/Reject)
    await expect(
      page.getByRole("button", { name: "Dismiss" }),
    ).toBeVisible();

    // Click "Dismiss"
    await page.getByRole("button", { name: "Dismiss" }).click();

    // Resolved banner appears
    await expect(page.getByText("Resolved").first()).toBeVisible({ timeout: 5000 });

    // ── Resolve the scaling decision ──
    // Navigate back to Inbox
    await page.getByRole("navigation").getByRole("link", { name: "Inbox" }).click();

    // Click on decision title → detail page
    await page
      .getByRole("link", { name: "Auto-scale cluster beyond budget limit?" })
      .click();

    // Category: "Decision". Resolve buttons: "Answered", "Dismissed"
    await expect(page.getByText("Decision")).toBeVisible({ timeout: 10000 });
    await expect(
      page.getByRole("button", { name: "Answered" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Dismissed" }),
    ).toBeVisible();

    // Click "Answered"
    await page.getByRole("button", { name: "Answered" }).click();

    // Fill response note
    await page
      .locator("#response-note")
      .fill("Approved temporary scale-up for 4 hours. Monitor costs closely.");

    // Click "Submit Resolution"
    await page.getByRole("button", { name: "Submit Resolution" }).click();

    // Resolved banner appears
    await expect(page.getByText("Resolved").first()).toBeVisible({ timeout: 5000 });

    // ══════════════════════════════════════════════
    // Act 2 — Incident Issue Management
    // ══════════════════════════════════════════════

    // Navigate to Issues via sidebar
    await page.getByRole("link", { name: "Issues" }).click();

    // Click "Create Issue" → dialog
    await page.getByRole("button", { name: "Create Issue" }).click();

    // Fill Title
    await page.locator("#issue-title").fill("INC-P1: API Latency Degradation");

    // Select Type: "Task" and Priority: "Critical" via comboboxes
    const issueComboboxes = page.getByRole("dialog").getByRole("combobox");
    // Order: Type(0), Status(1), Priority(2), Assignee(3), Project(4), Goal(5)
    await issueComboboxes.nth(0).click();
    await page.getByRole("option", { name: "Task" }).click();

    await issueComboboxes.nth(2).click();
    await page.getByRole("option", { name: "Critical" }).click();

    // Submit
    await page.getByRole("dialog").getByRole("button", { name: "Save" }).click();

    // "SRE-1" appears
    await expect(page.getByText("SRE-1")).toBeVisible({ timeout: 10000 });

    // Click on the issue title → detail page
    await page.getByRole("link", { name: "INC-P1: API Latency Degradation" }).click();

    // Move through statuses: click "Todo", then "In Progress"
    await page.getByRole("button", { name: "Todo" }).click();
    await page.getByRole("button", { name: "In Progress" }).click();

    // Type comment and add
    await page
      .getByPlaceholder("Add a comment...")
      .fill(
        "14:32 - Alert triggered. API p99 latency exceeded 5s threshold.",
      );
    await page.getByRole("button", { name: "Add Comment" }).click();

    // Comment appears
    await expect(
      page.getByText(
        "14:32 - Alert triggered. API p99 latency exceeded 5s threshold.",
      ),
    ).toBeVisible({ timeout: 5000 });

    // Wait for textarea to clear after first comment was added
    await expect(page.getByPlaceholder("Add a comment...")).toHaveValue("", { timeout: 5000 });

    // Type second comment
    await page
      .getByPlaceholder("Add a comment...")
      .fill(
        "14:35 - Triager identified root cause: database connection pool exhaustion.",
      );
    // Wait for button to become enabled
    await expect(page.getByRole("button", { name: "Add Comment" })).toBeEnabled({ timeout: 5000 });
    await page.getByRole("button", { name: "Add Comment" }).click();

    // Both comments visible
    await expect(
      page.getByText(
        "14:32 - Alert triggered. API p99 latency exceeded 5s threshold.",
      ),
    ).toBeVisible();
    await expect(
      page.getByText(
        "14:35 - Triager identified root cause: database connection pool exhaustion.",
      ),
    ).toBeVisible({ timeout: 5000 });

    // ══════════════════════════════════════════════
    // Act 3 — Incident Coordination Conversation
    // ══════════════════════════════════════════════

    // Navigate to Conversations via sidebar
    await page.getByRole("link", { name: "Conversations" }).click();

    // Click "New Conversation" → dialog
    await page.getByRole("button", { name: "New Conversation" }).click();

    // Select agent: "Alert Triager" (from dropdown)
    const convDialog = page.getByRole("dialog");
    await convDialog.getByRole("combobox").click();
    await page.getByRole("option", { name: "Alert Triager" }).click();

    // Title
    await page
      .locator("#conv-title")
      .fill("Coordinate API latency incident response");

    // First message
    await page
      .locator("#conv-message")
      .fill("What's the current connection pool status?");

    // Click "Start"
    await page.getByRole("button", { name: "Start" }).click();

    // Redirected to conversation page — see title and first message bubble
    await expect(
      page.getByText("Coordinate API latency incident response"),
    ).toBeVisible({ timeout: 10000 });
    await expect(
      page.getByText("What's the current connection pool status?"),
    ).toBeVisible();

    // Type second message in textarea, press Enter
    await page
      .getByPlaceholder("Type a message...")
      .fill("Can you also check the replica lag?");
    await page.getByPlaceholder("Type a message...").press("Enter");

    // Second message appears
    await expect(
      page.getByText("Can you also check the replica lag?"),
    ).toBeVisible({ timeout: 5000 });

    // Click "Close" button
    await page.getByRole("button", { name: "Close" }).click();

    // "This conversation is closed." appears
    await expect(
      page.getByText("This conversation is closed."),
    ).toBeVisible({ timeout: 5000 });

    // ══════════════════════════════════════════════
    // Act 4 — Agent Status Management
    // ══════════════════════════════════════════════

    // Navigate to Agents via sidebar
    await page.getByRole("link", { name: "Agents" }).click();

    // See 3 agents
    await expect(page.getByText("Incident Commander")).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByText("Alert Triager")).toBeVisible();
    await expect(page.getByText("Runbook Executor")).toBeVisible();

    // Click on "Alert Triager" → detail page
    await page.getByRole("link", { name: "Alert Triager" }).click();

    // See "Pause" button (agent is active)
    await expect(
      page.getByRole("button", { name: "Pause" }),
    ).toBeVisible({ timeout: 10000 });

    // Click "Pause" → Resume button appears
    await page.getByRole("button", { name: "Pause" }).click();
    await expect(page.getByRole("button", { name: "Resume" })).toBeVisible({ timeout: 5000 });

    // Click "Resume" → Pause button returns
    await page.getByRole("button", { name: "Resume" }).click();
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible({ timeout: 5000 });

    // Click "Edit" → edit mode
    await page.getByRole("button", { name: "Edit" }).click();

    // Change Name field to "Senior Alert Triager"
    await page.locator("#edit-name").clear();
    await page.locator("#edit-name").fill("Senior Alert Triager");

    // Click "Save"
    await page.getByRole("button", { name: "Save" }).click();

    // See updated name in the heading
    await expect(page.getByRole("heading", { name: "Senior Alert Triager" })).toBeVisible({
      timeout: 5000,
    });

    // ══════════════════════════════════════════════
    // Act 5 — Dashboard Verification
    // ══════════════════════════════════════════════

    // Navigate to Dashboard
    await page.getByRole("link", { name: "Dashboard" }).click();

    // Stat cards visible
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "Active Agents" }),
    ).toBeVisible({ timeout: 10000 });
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "In Progress" }),
    ).toBeVisible();
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "Open Issues" }),
    ).toBeVisible();
  });
});
