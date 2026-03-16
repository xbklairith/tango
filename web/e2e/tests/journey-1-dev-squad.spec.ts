import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";

test.describe("Journey 1: Ship a Feature Sprint", () => {
  test("full lifecycle: squad → agents → issues → conversations → dashboard", async ({
    page,
    apiContext,
  }) => {
    const email = `j1-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    // Register via API, then login via UI
    await registerViaAPI(apiContext, email, "Journey One User", password);
    const cookies = await loginViaAPI(apiContext, email, password);
    await new Promise((r) => setTimeout(r, 1100));

    // ──────────────────────────────────────────────────────────
    // Act 1 — Squad Creation (UI wizard)
    // ──────────────────────────────────────────────────────────

    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    // Dashboard shows "Welcome to Ari" empty state
    await expect(page.getByText("Welcome to Ari")).toBeVisible({ timeout: 10000 });
    await expect(page.getByRole("button", { name: "Create Squad" })).toBeVisible();

    // Click "Create Squad" → dialog opens on Step 1
    await page.getByRole("button", { name: "Create Squad" }).click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Step 1 of 2")).toBeVisible();

    // Fill squad details
    await dialog.locator("#squad-name").fill("Frontend Team");
    await dialog.locator("#squad-prefix").fill("FE");
    await dialog.locator("#squad-desc").fill("Frontend engineering squad");

    // Click "Next" → Step 2
    await dialog.getByRole("button", { name: "Next" }).click();
    await expect(dialog.getByText("Step 2 of 2")).toBeVisible();

    // Fill captain details
    await dialog.locator("#captain-name").fill("Orchestrator");
    await dialog.locator("#captain-short").fill("orchestrator");

    // Click "Create Squad" → dialog closes
    await dialog.getByRole("button", { name: "Create Squad" }).click();
    await expect(dialog).not.toBeVisible({ timeout: 10000 });

    // After squad creation, navigate to dashboard to see stat cards
    await page.getByRole("link", { name: "Dashboard" }).click();
    await expect(page.getByRole("heading", { name: "Frontend Team" })).toBeVisible({ timeout: 10000 });
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "Active Agents" }),
    ).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 2 — Agent Creation (UI)
    // ──────────────────────────────────────────────────────────

    // Enable "require approval for new agents" via API so we can test the approval flow
    // Get the squad ID from the user's squads via auth/me
    const meResp = await apiContext.get("/api/auth/me", { headers: { Cookie: cookies } });
    const me = await meResp.json();
    const squadId = me.squads[0].squadId;
    await apiContext.patch(`/api/squads/${squadId}`, {
      data: { settings: { requireApprovalForNewAgents: true } },
      headers: { Cookie: cookies },
    });

    // Navigate to Agents via sidebar
    await page.getByRole("link", { name: "Agents" }).click();
    await page.waitForURL(/\/agents/, { timeout: 10000 });

    // Captain "Orchestrator" should be visible in table
    await expect(page.getByText("Orchestrator")).toBeVisible({ timeout: 10000 });

    // Create first agent: Code Reviewer (Lead)
    await page.getByRole("button", { name: "Create Agent" }).click();
    const agentDialog = page.getByRole("dialog");
    await expect(agentDialog).toBeVisible();

    await agentDialog.locator("#agent-name").fill("Code Reviewer");
    await agentDialog.locator("#agent-urlkey").fill("code-reviewer");

    // Select Role = Lead (shadcn Select)
    const roleSelect = agentDialog.locator("label:has-text('Role') + div button, label:has-text('Role') ~ *  button[role='combobox']").first();
    // Alternative approach: find the role select by looking at all comboboxes in dialog
    const comboboxes = agentDialog.getByRole("combobox");
    // Role is the first select, Reports To is the second
    await comboboxes.nth(0).click();
    await page.getByRole("option", { name: "Lead" }).click();

    // Select Reports To = Orchestrator
    await comboboxes.nth(1).click();
    await page.getByRole("option", { name: "Orchestrator" }).click();

    // Submit
    await agentDialog.getByRole("button", { name: "Save" }).click();
    await expect(agentDialog).not.toBeVisible({ timeout: 10000 });

    // Agent list should show 2 agents
    await expect(page.getByText("Code Reviewer")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Orchestrator")).toBeVisible();

    // Create second agent: Junior Coder (Member)
    await page.getByRole("button", { name: "Create Agent" }).click();
    const agentDialog2 = page.getByRole("dialog");
    await expect(agentDialog2).toBeVisible();

    await agentDialog2.locator("#agent-name").fill("Junior Coder");
    await agentDialog2.locator("#agent-urlkey").fill("junior-coder");

    const comboboxes2 = agentDialog2.getByRole("combobox");
    await comboboxes2.nth(0).click();
    await page.getByRole("option", { name: "Member" }).click();

    await comboboxes2.nth(1).click();
    await page.getByRole("option", { name: "Code Reviewer" }).click();

    await agentDialog2.getByRole("button", { name: "Save" }).click();
    await expect(agentDialog2).not.toBeVisible({ timeout: 10000 });

    // Agent list should show 3 agents
    await expect(page.getByText("Junior Coder")).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 3 — Agent Approval
    // ──────────────────────────────────────────────────────────

    // Click on "Code Reviewer" → agent detail page
    await page.getByRole("link", { name: "Code Reviewer" }).click();
    await page.waitForURL(/\/agents\//, { timeout: 10000 });

    // Approve button visible (agent is pending approval)
    await expect(page.getByRole("button", { name: "Approve" })).toBeVisible({ timeout: 10000 });

    // Click "Approve" → Pause button appears (indicating active)
    await page.getByRole("button", { name: "Approve" }).click();
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible({ timeout: 10000 });

    // Navigate back to agents list
    await page.goBack();
    await page.waitForURL(/\/agents/, { timeout: 10000 });

    // Click on "Junior Coder" → approve same way
    await page.getByRole("link", { name: "Junior Coder" }).click();
    await page.waitForURL(/\/agents\//, { timeout: 10000 });

    await expect(page.getByRole("button", { name: "Approve" })).toBeVisible({ timeout: 10000 });
    await page.getByRole("button", { name: "Approve" }).click();
    await expect(page.getByRole("button", { name: "Pause" })).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 4 — Issue Creation & Lifecycle
    // ──────────────────────────────────────────────────────────

    // Navigate to Issues via sidebar
    await page.getByRole("link", { name: "Issues" }).click();
    await page.waitForURL(/\/issues/, { timeout: 10000 });

    // Create first issue
    await page.getByRole("button", { name: "Create Issue" }).click();
    const issueDialog = page.getByRole("dialog");
    await expect(issueDialog).toBeVisible();

    await issueDialog.locator("#issue-title").fill("Design new auth flow");

    // Select Type = Task
    const issueComboboxes = issueDialog.getByRole("combobox");
    // Order in create-issue-dialog: Type, Status, Priority, Assignee, Project, Goal
    await issueComboboxes.nth(0).click();
    await page.getByRole("option", { name: "Task" }).click();

    // Select Priority = Critical
    await issueComboboxes.nth(2).click();
    await page.getByRole("option", { name: "Critical" }).click();

    // Select Assignee = Code Reviewer
    await issueComboboxes.nth(3).click();
    await page.getByRole("option", { name: "Code Reviewer" }).click();

    // Submit
    await issueDialog.getByRole("button", { name: "Save" }).click();
    await expect(issueDialog).not.toBeVisible({ timeout: 10000 });

    // "FE-1" appears in list
    await expect(page.getByText("FE-1")).toBeVisible({ timeout: 10000 });

    // Create second issue
    await page.getByRole("button", { name: "Create Issue" }).click();
    const issueDialog2 = page.getByRole("dialog");
    await expect(issueDialog2).toBeVisible();

    await issueDialog2.locator("#issue-title").fill("Implement token refresh");

    const issueComboboxes2 = issueDialog2.getByRole("combobox");

    // Select Priority = High
    await issueComboboxes2.nth(2).click();
    await page.getByRole("option", { name: "High" }).click();

    // Select Assignee = Junior Coder
    await issueComboboxes2.nth(3).click();
    await page.getByRole("option", { name: "Junior Coder" }).click();

    // Submit
    await issueDialog2.getByRole("button", { name: "Save" }).click();
    await expect(issueDialog2).not.toBeVisible({ timeout: 10000 });

    // "FE-2" appears in list
    await expect(page.getByText("FE-2")).toBeVisible({ timeout: 10000 });

    // Click on "FE-1" → detail page
    await page.getByRole("link", { name: "Design new auth flow" }).click();
    await page.waitForURL(/\/issues\//, { timeout: 10000 });

    // See status "Backlog" in the status card, and priority "Critical"
    await expect(page.locator(".grid.gap-4 .rounded-lg.border").first().getByText("Backlog")).toBeVisible({ timeout: 10000 });
    await expect(page.locator(".grid.gap-4 .rounded-lg.border").nth(1).getByText("Critical")).toBeVisible();

    // Status transitions: click "Todo" → then "In Progress"
    await page.getByRole("button", { name: "Todo" }).click();
    await expect(page.locator(".grid.gap-4 .rounded-lg.border").first().getByText("Todo")).toBeVisible({ timeout: 10000 });

    await page.getByRole("button", { name: "In Progress" }).click();
    await expect(page.locator(".grid.gap-4 .rounded-lg.border").first().getByText("In Progress")).toBeVisible({ timeout: 10000 });

    // Add a comment
    await page.getByPlaceholder("Add a comment...").fill("Starting the design work now");
    await page.getByRole("button", { name: "Add Comment" }).click();
    await expect(page.getByText("Starting the design work now")).toBeVisible({ timeout: 10000 });

    // Edit the issue
    await page.getByRole("button", { name: "Edit" }).click();

    // Change title
    await page.locator("#edit-title").clear();
    await page.locator("#edit-title").fill("Design new auth flow (Revised)");

    // Change priority to High
    await page.getByRole("combobox").click();
    await page.getByRole("option", { name: "High" }).click();

    // Save
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("Design new auth flow (Revised)")).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 5 — Conversations
    // ──────────────────────────────────────────────────────────

    // Navigate to Conversations via sidebar
    await page.getByRole("link", { name: "Conversations" }).click();
    await page.waitForURL(/\/conversations/, { timeout: 10000 });

    // See empty state
    await expect(page.getByText("No conversations yet")).toBeVisible({ timeout: 10000 });

    // Click "New Conversation"
    await page.getByRole("button", { name: "New Conversation" }).click();
    const convDialog = page.getByRole("dialog");
    await expect(convDialog).toBeVisible();

    // Select agent: Code Reviewer (only active agents shown)
    await convDialog.getByRole("combobox").click();
    await page.getByRole("option", { name: "Code Reviewer" }).click();

    // Fill title and message
    await convDialog.locator("#conv-title").fill("Discuss API contract for auth");
    await convDialog.locator("#conv-message").fill("What's your recommendation for the token format?");

    // Click "Start"
    await convDialog.getByRole("button", { name: "Start" }).click();
    await expect(convDialog).not.toBeVisible({ timeout: 10000 });

    // Redirected to conversation page — see title and user message
    await page.waitForURL(/\/conversations\//, { timeout: 10000 });
    await expect(page.getByText("Discuss API contract for auth")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("What's your recommendation for the token format?")).toBeVisible({ timeout: 10000 });

    // Send a second message
    await page.getByPlaceholder("Type a message...").fill("Also, should we support refresh tokens?");
    await page.getByPlaceholder("Type a message...").press("Enter");
    await expect(page.getByText("Also, should we support refresh tokens?")).toBeVisible({ timeout: 10000 });

    // Close the conversation
    await page.getByRole("button", { name: "Close" }).click();
    await expect(page.getByText("This conversation is closed.")).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 6 — Dashboard Verification
    // ──────────────────────────────────────────────────────────

    // Navigate to Dashboard via sidebar
    await page.getByRole("link", { name: "Dashboard" }).click();
    await page.waitForURL(/\/$/, { timeout: 10000 });

    // Verify stat cards exist
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "Active Agents" }),
    ).toBeVisible({ timeout: 10000 });
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "In Progress" }),
    ).toBeVisible();
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "Open Issues" }),
    ).toBeVisible();
    await expect(
      page.locator('[data-slot="card-title"]').filter({ hasText: "Projects" }),
    ).toBeVisible();
  });
});
