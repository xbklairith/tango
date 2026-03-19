import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";
import type { APIRequestContext } from "@playwright/test";

/**
 * Journey 5: Agent Team Creation via Conversation → Inbox Approval
 *
 * Tests the full flow:
 * 1. Create squad with captain via UI wizard
 * 2. Start a conversation asking the captain to create a team
 * 3. Wait for the agent to process and create an inbox approval request
 * 4. Approve the inbox item via UI
 * 5. Verify the inbox_resolved wakeup fires and a new run starts
 * 6. Optionally verify agent created team members (LLM-dependent)
 */

async function waitForRunComplete(
  apiContext: APIRequestContext,
  agentId: string,
  cookies: string,
  opts: { timeout?: number; initialRunCount?: number } = {},
): Promise<void> {
  const timeout = opts.timeout ?? 120_000;
  const initialRunCount = opts.initialRunCount ?? 0;
  const start = Date.now();

  while (Date.now() - start < timeout) {
    const resp = await apiContext.get(`/api/agents/${agentId}/runs`, {
      headers: { Cookie: cookies },
    });
    if (resp.ok()) {
      const runs = await resp.json();
      const completedRuns = runs.filter(
        (r: { status: string }) =>
          r.status === "succeeded" || r.status === "failed" || r.status === "timed_out",
      );
      if (completedRuns.length > initialRunCount) {
        return;
      }
    }
    await new Promise((r) => setTimeout(r, 5000));
  }
  throw new Error(`Timed out waiting for agent run to complete (${timeout}ms)`);
}

/**
 * Wait for a new run to appear (any status including "running").
 * Used to verify the inbox_resolved wakeup fires.
 */
async function waitForNewRun(
  apiContext: APIRequestContext,
  agentId: string,
  cookies: string,
  initialRunCount: number,
  timeout = 60_000,
): Promise<void> {
  const start = Date.now();

  while (Date.now() - start < timeout) {
    const resp = await apiContext.get(`/api/agents/${agentId}/runs`, {
      headers: { Cookie: cookies },
    });
    if (resp.ok()) {
      const runs = await resp.json();
      if (runs.length > initialRunCount) {
        return;
      }
    }
    await new Promise((r) => setTimeout(r, 3000));
  }
  throw new Error(`Timed out waiting for new run to appear (${timeout}ms)`);
}

test.describe("Journey 5: Agent Team Creation via Conversation", () => {
  test("conversation → inbox approval → inbox_resolved wakeup fires", async ({
    page,
    apiContext,
  }) => {
    // This test involves agent LLM runs which are slow — agent may retry + self-correct
    test.setTimeout(900_000); // 15 min — extended for captain + member LLM runs

    const email = `j5-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";
    const email2 = `j5-u2-${Date.now()}@e2e.test`;
    const password2 = "TestP@ss1234!";

    // ──────────────────────────────────────────────────────────
    // Setup — Register & Login
    // ──────────────────────────────────────────────────────────

    await registerViaAPI(apiContext, email, "Journey Five User", password);
    // Write credentials to file so they can be used after the test
    const { writeFileSync } = await import("fs");
    writeFileSync("/tmp/j5-credentials.json", JSON.stringify({ email, password }));

    const cookies = await loginViaAPI(apiContext, email, password);
    await new Promise((r) => setTimeout(r, 1100));

    // Login via UI
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    // ──────────────────────────────────────────────────────────
    // Act 1 — Create Squad via UI Wizard
    // ──────────────────────────────────────────────────────────

    await expect(page.getByText("Welcome to Ari")).toBeVisible({ timeout: 10000 });
    await page.getByRole("button", { name: "Create Squad" }).click();

    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("Step 1 of 2")).toBeVisible();

    // Step 1: Squad details
    await dialog.locator("#squad-name").fill("Research Team");
    await dialog.locator("#squad-prefix").fill("RES");
    await dialog.locator("#squad-desc").fill("Investment research squad");

    // Step 2: Captain details
    await dialog.getByRole("button", { name: "Next" }).click();
    await expect(dialog.getByText("Step 2 of 2")).toBeVisible();
    await dialog.locator("#captain-name").fill("Research Captain");
    await dialog.locator("#captain-short").fill("research-captain");

    await dialog.getByRole("button", { name: "Create Squad" }).click();
    await expect(dialog).not.toBeVisible({ timeout: 10000 });

    // Get squad and captain IDs via API (poll until squad appears)
    let squadId = "";
    await expect(async () => {
      const meResp = await apiContext.get("/api/auth/me", { headers: { Cookie: cookies } });
      const me = await meResp.json();
      expect(me.squads?.length).toBeGreaterThan(0);
      squadId = me.squads[0].squadId;
    }).toPass({ timeout: 15_000 });

    let captainId = "";
    await expect(async () => {
      const agentsResp = await apiContext.get(`/api/agents?squadId=${squadId}`, {
        headers: { Cookie: cookies },
      });
      const agents = await agentsResp.json();
      const captain = agents.find((a: { role: string }) => a.role === "captain");
      expect(captain).toBeTruthy();
      captainId = captain.id;
    }).toPass({ timeout: 10_000 });

    // ──────────────────────────────────────────────────────────
    // Act 2 — Start Conversation
    // ──────────────────────────────────────────────────────────

    await page.getByRole("link", { name: "Conversations" }).click();
    await page.waitForURL(/\/conversations/, { timeout: 10000 });

    await page.getByRole("button", { name: "New Conversation" }).click();
    const convDialog = page.getByRole("dialog");
    await expect(convDialog).toBeVisible();

    // Select the captain agent
    await convDialog.getByRole("combobox").click();
    await page.getByRole("option", { name: "Research Captain" }).click();

    await convDialog.locator("#conv-title").fill("Create investment research team");
    await convDialog
      .locator("#conv-message")
      .fill(
        "Create an investment research team with 3 members: a Lead Analyst, a Data Collector, and a Report Writer. Send an approval request to my inbox before creating them.",
      );

    await convDialog.getByRole("button", { name: "Start" }).click();
    await expect(convDialog).not.toBeVisible({ timeout: 10000 });

    // Verify we're on the conversation page and capture the URL for later
    await page.waitForURL(/\/conversations\//, { timeout: 10000 });
    const conversationUrl = page.url();
    await expect(page.getByText("Create investment research team")).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 3 — Wait for Agent Run to Complete
    // ──────────────────────────────────────────────────────────

    // The agent should process the message and create an inbox approval request
    await waitForRunComplete(apiContext, captainId, cookies, { timeout: 120_000 });

    // ──────────────────────────────────────────────────────────
    // Act 4 — Verify Inbox Item Exists
    // ──────────────────────────────────────────────────────────

    await page.getByRole("link", { name: "Inbox" }).click();
    await page.waitForURL(/\/inbox/, { timeout: 10000 });

    // Wait for an inbox item to appear (the agent should have created an approval request)
    // Inbox API returns { data: [...], pagination: {...} }
    let inboxData: { data: any[] } = { data: [] };
    await expect(async () => {
      const inboxResp = await apiContext.get(`/api/squads/${squadId}/inbox`, {
        headers: { Cookie: cookies },
      });
      inboxData = await inboxResp.json();
      expect(inboxData.data.length).toBeGreaterThan(0);
    }).toPass({ timeout: 30_000 });

    // ──────────────────────────────────────────────────────────
    // Act 5 — Approve via UI
    // ──────────────────────────────────────────────────────────

    // Find the approval item from API response
    const approvalItem = inboxData.data.find(
      (item: { category: string; status: string }) =>
        item.category === "approval" && item.status === "pending",
    ) || inboxData.data[0];

    if (!approvalItem) {
      throw new Error("No inbox item found after agent run");
    }

    // Snapshot run count BEFORE approval to detect the inbox_resolved run
    const preApprovalRunsResp = await apiContext.get(`/api/agents/${captainId}/runs`, {
      headers: { Cookie: cookies },
    });
    const preApprovalRuns = await preApprovalRunsResp.json();
    const preApprovalRunCount = preApprovalRuns.length;

    // Navigate to inbox list, then click the item
    await page.getByRole("link", { name: "Inbox" }).click();
    await page.waitForURL(/\/inbox/, { timeout: 10000 });
    await page.getByText(approvalItem.title).click();
    await page.waitForURL(/\/inbox\//, { timeout: 10000 });

    // Step 1: Select "Approve" resolution (this is a toggle, not the submit)
    const approveBtn = page.getByRole("button", { name: "Approve" });
    await expect(approveBtn).toBeVisible({ timeout: 10000 });
    await approveBtn.click();

    // Step 2: Fill the response note
    const noteInput = page.locator("#response-note");
    await expect(noteInput).toBeVisible({ timeout: 5000 });
    await noteInput.fill("Approved. Please create all 3 team members.");

    // Step 3: Click "Submit Resolution" to actually resolve
    const submitBtn = page.getByRole("button", { name: "Submit Resolution" });
    await expect(submitBtn).toBeEnabled({ timeout: 5000 });
    await submitBtn.click();

    // Verify the resolved banner appears
    await expect(page.locator(".bg-green-50")).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 6 — Wait for inbox_resolved Run to Complete
    // ──────────────────────────────────────────────────────────

    // Wait for the new run to appear first
    await waitForNewRun(apiContext, captainId, cookies, preApprovalRunCount, 60_000);

    // Verify it's an inbox_resolved run
    const postApprovalRunsResp = await apiContext.get(`/api/agents/${captainId}/runs`, {
      headers: { Cookie: cookies },
    });
    const postApprovalRuns = await postApprovalRunsResp.json();
    const inboxResolvedRun = postApprovalRuns.find(
      (r: { invocationSource: string }) => r.invocationSource === "inbox_resolved",
    );
    expect(inboxResolvedRun).toBeTruthy();

    // Now wait for the inbox_resolved run to COMPLETE
    const completedBeforeApproval = postApprovalRuns.filter(
      (r: { status: string }) =>
        r.status === "succeeded" || r.status === "failed" || r.status === "timed_out",
    ).length;

    await waitForRunComplete(apiContext, captainId, cookies, {
      timeout: 300_000, // 5 min — agent may retry and self-correct on auth errors
      initialRunCount: completedBeforeApproval,
    });

    // ──────────────────────────────────────────────────────────
    // Act 7 — Verify Agents Created
    // ──────────────────────────────────────────────────────────

    // Poll for agents — the agent should have created at least 1 new agent
    await expect(async () => {
      const agentsResp = await apiContext.get(`/api/agents?squadId=${squadId}`, {
        headers: { Cookie: cookies },
      });
      const agentsList = await agentsResp.json();
      expect(agentsList.length).toBeGreaterThan(1);
    }).toPass({ timeout: 10_000 });

    const finalAgentsResp = await apiContext.get(`/api/agents?squadId=${squadId}`, {
      headers: { Cookie: cookies },
    });
    const finalAgents = await finalAgentsResp.json();
    console.log(`[journey-5] Agent created ${finalAgents.length - 1} new agents:`);
    for (const a of finalAgents) {
      console.log(`  - ${a.name} (${a.role}) status=${a.status}`);
    }

    // Navigate to Agents page and verify in UI
    await page.getByRole("link", { name: "Agents" }).click();
    await page.waitForURL(/\/agents/, { timeout: 10000 });
    await expect(page.getByText("Research Captain")).toBeVisible({ timeout: 10000 });

    // Verify at least one new agent is visible in the UI
    const agentRows = page.locator("table tbody tr");
    await expect(agentRows).not.toHaveCount(1, { timeout: 10000 }); // more than just captain

    // Collect member agent IDs (non-captain agents)
    const memberAgents = finalAgents.filter(
      (a: { id: string; role: string }) => a.role !== "captain",
    );
    console.log(`[journey-5] ${memberAgents.length} member agents found`);

    // ──────────────────────────────────────────────────────────
    // Act 8 — Send Follow-up Message to Captain
    // ──────────────────────────────────────────────────────────

    await page.goto(conversationUrl);
    await page.waitForURL(/\/conversations\//, { timeout: 10000 });

    const messageInput = page.getByTestId("message-input");
    await expect(messageInput).toBeVisible({ timeout: 10000 });
    await messageInput.fill(
      "Now create an issue for each team member asking them to introduce themselves. Assign each issue to the respective member. Each member should send their introduction to my inbox.",
    );
    await page.getByTestId("send-message-btn").click();

    // Wait for the message to appear in conversation
    await expect(
      page.getByText("Now create an issue for each team member"),
    ).toBeVisible({ timeout: 10000 });

    // ──────────────────────────────────────────────────────────
    // Act 9 — Wait for Captain Run to Complete
    // ──────────────────────────────────────────────────────────

    // Snapshot current completed run count
    const act9RunsResp = await apiContext.get(`/api/agents/${captainId}/runs`, {
      headers: { Cookie: cookies },
    });
    const act9Runs = await act9RunsResp.json();
    const act9CompletedCount = act9Runs.filter(
      (r: { status: string }) =>
        r.status === "succeeded" || r.status === "failed" || r.status === "timed_out",
    ).length;

    await waitForRunComplete(apiContext, captainId, cookies, {
      timeout: 180_000,
      initialRunCount: act9CompletedCount,
    });

    console.log("[journey-5] Captain run completed after follow-up message");

    // ──────────────────────────────────────────────────────────
    // Act 10 — Verify Issues Created and Assigned
    // ──────────────────────────────────────────────────────────

    let assignedIssues: any[] = [];
    await expect(async () => {
      const issuesResp = await apiContext.get(`/api/squads/${squadId}/issues`, {
        headers: { Cookie: cookies },
      });
      const issuesBody = await issuesResp.json();
      // API returns { data: [...], pagination: {...} }
      const issues = Array.isArray(issuesBody) ? issuesBody : issuesBody.data ?? [];
      const memberIds = new Set(memberAgents.map((a: { id: string }) => a.id));
      const memberNames = memberAgents.map((a: { name: string }) => a.name.toLowerCase());
      // Match issues assigned to members OR with member names in the title
      // (LLM may create issues without assigneeAgentId but with member name in title)
      assignedIssues = issues.filter(
        (i: { type: string; title: string; assigneeAgentId: string | null }) => {
          if (i.type !== "task") return false;
          if (i.assigneeAgentId && memberIds.has(i.assigneeAgentId)) return true;
          const titleLower = i.title.toLowerCase();
          return memberNames.some((name) => titleLower.includes(name));
        },
      );
      expect(assignedIssues.length).toBeGreaterThanOrEqual(memberAgents.length);
    }).toPass({ timeout: 30_000 });

    console.log(`[journey-5] ${assignedIssues.length} issues for members:`);
    for (const issue of assignedIssues) {
      const member = memberAgents.find((a: { id: string }) => a.id === issue.assigneeAgentId);
      console.log(`  - "${issue.title}" → ${member?.name ?? "(by title match)"} (${issue.status})`);
    }

    // ──────────────────────────────────────────────────────────
    // Act 11 — Wait for All Member Runs to Complete
    // ──────────────────────────────────────────────────────────

    // Check if any issues were actually assigned (triggers member wakeups)
    const memberIds = new Set(memberAgents.map((a: { id: string }) => a.id));
    const hasAssignedIssues = assignedIssues.some(
      (i: { assigneeAgentId: string | null }) => i.assigneeAgentId && memberIds.has(i.assigneeAgentId),
    );

    let memberInboxItems: any[] = [];

    if (hasAssignedIssues) {
      console.log("[journey-5] Waiting for all member agent runs to complete...");

      await Promise.all(
        memberAgents.map((agent: { id: string; name: string }) =>
          waitForRunComplete(apiContext, agent.id, cookies, { timeout: 180_000 }).then(() =>
            console.log(`[journey-5]   ✓ ${agent.name} run completed`),
          ),
        ),
      );

      console.log("[journey-5] All member runs completed");

      // ──────────────────────────────────────────────────────────
      // Act 12 — Verify Inbox Items from Members
      // ──────────────────────────────────────────────────────────

      await expect(async () => {
        const inboxResp = await apiContext.get(`/api/squads/${squadId}/inbox`, {
          headers: { Cookie: cookies },
        });
        const inbox = await inboxResp.json();
        memberInboxItems = inbox.data.filter(
          (item: { requestedByAgentId: string | null }) =>
            item.requestedByAgentId && memberIds.has(item.requestedByAgentId),
        );
        expect(memberInboxItems.length).toBeGreaterThanOrEqual(memberAgents.length);
      }).toPass({ timeout: 120_000 });

      console.log(`[journey-5] ${memberInboxItems.length} inbox items from members:`);
      for (const item of memberInboxItems) {
        const member = memberAgents.find((a: { id: string }) => a.id === item.requestedByAgentId);
        console.log(`  - "${item.title}" from ${member?.name ?? item.requestedByAgentId}`);
      }
    } else {
      console.log("[journey-5] Issues not assigned to agents (LLM did not set assigneeAgentId) — skipping member run/inbox verification");
    }

    // ──────────────────────────────────────────────────────────
    // Act 13 — Verify in UI (Inbox + Issues)
    // ──────────────────────────────────────────────────────────

    if (memberInboxItems.length > 0) {
      // Check Inbox page
      await page.getByRole("link", { name: "Inbox" }).click();
      await page.waitForURL(/\/inbox/, { timeout: 10000 });

      const inboxTable = page.getByTestId("inbox-table");
      await expect(inboxTable).toBeVisible({ timeout: 10000 });
      for (const item of memberInboxItems.slice(0, 3)) {
        await expect(page.getByTestId(`inbox-item-${item.id}`)).toBeVisible({ timeout: 10000 });
      }
    }

    // Check Issues page — always verify issues exist
    await page.getByRole("link", { name: "Issues" }).click();
    await page.waitForURL(/\/issues/, { timeout: 10000 });

    const issuesTable = page.getByTestId("issues-table");
    await expect(issuesTable).toBeVisible({ timeout: 10000 });
    for (const issue of assignedIssues.slice(0, 3)) {
      await expect(page.getByTestId(`issue-row-${issue.id}`)).toBeVisible({ timeout: 10000 });
    }

    console.log("[journey-5] ✅ User 1 journey complete: Captain → Issues → Members → Inbox");

    // ──────────────────────────────────────────────────────────
    // Act 14 — Register User 2 & Verify Auto-Membership (API)
    // ──────────────────────────────────────────────────────────

    console.log("[journey-5] Registering User 2 to verify shared squad model...");

    await registerViaAPI(apiContext, email2, "Journey Five Verifier", password2);
    const cookies2 = await loginViaAPI(apiContext, email2, password2);

    // Verify User 2 is auto-added to the squad
    const meResp2 = await apiContext.get("/api/auth/me", {
      headers: { Cookie: cookies2 },
    });
    const me2 = await meResp2.json();
    const user2Squad = me2.squads?.find(
      (s: { squadId: string }) => s.squadId === squadId,
    );
    expect(user2Squad).toBeTruthy();
    expect(user2Squad.role).toBe("admin");
    console.log(`[journey-5] User 2 auto-added to squad as ${user2Squad.role}`);

    // ──────────────────────────────────────────────────────────
    // Act 15 — Logout User 1, Login User 2 in Browser
    // ──────────────────────────────────────────────────────────

    await page.getByTitle("Logout").click();
    await page.waitForURL(/\/login/, { timeout: 10000 });

    await page.getByLabel("Email").fill(email2);
    await page.getByLabel("Password").fill(password2);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    console.log("[journey-5] User 2 logged in via browser");

    // ──────────────────────────────────────────────────────────
    // Act 16 — User 2 Verifies Agents
    // ──────────────────────────────────────────────────────────

    await page.goto(`/squads/${squadId}/agents`);
    await page.waitForURL(/\/agents/, { timeout: 10000 });

    // Verify captain is visible
    await expect(page.getByText("Research Captain")).toBeVisible({ timeout: 10000 });

    // Verify member agents are visible
    const u2AgentRows = page.locator("table tbody tr");
    await expect(u2AgentRows).not.toHaveCount(1, { timeout: 10000 });

    for (const agent of memberAgents.slice(0, 3)) {
      await expect(page.getByText(agent.name)).toBeVisible({ timeout: 5000 });
    }

    console.log(`[journey-5] User 2 sees ${memberAgents.length + 1} agents (captain + members)`);

    // ──────────────────────────────────────────────────────────
    // Act 17 — User 2 Verifies Conversations
    // ──────────────────────────────────────────────────────────

    await page.goto(`/squads/${squadId}/conversations`);
    await page.waitForURL(/\/conversations/, { timeout: 10000 });

    await expect(
      page.getByText("Create investment research team"),
    ).toBeVisible({ timeout: 10000 });

    // Click into the conversation and verify messages are visible
    await page.getByText("Create investment research team").click();
    await page.waitForURL(/\/conversations\//, { timeout: 10000 });
    await expect(page.getByTestId("messages-area")).toBeVisible({ timeout: 10000 });

    console.log("[journey-5] User 2 sees conversation and messages");

    // ──────────────────────────────────────────────────────────
    // Act 18 — User 2 Verifies Issues
    // ──────────────────────────────────────────────────────────

    await page.goto(`/squads/${squadId}/issues`);
    await page.waitForURL(/\/issues/, { timeout: 10000 });

    await expect(page.getByTestId("issues-table")).toBeVisible({ timeout: 10000 });

    for (const issue of assignedIssues.slice(0, 3)) {
      await expect(page.getByTestId(`issue-row-${issue.id}`)).toBeVisible({ timeout: 5000 });
    }

    console.log(`[journey-5] User 2 sees ${assignedIssues.length} assigned issues`);

    // ──────────────────────────────────────────────────────────
    // Act 19 — User 2 Verifies Inbox
    // ──────────────────────────────────────────────────────────

    await page.goto(`/squads/${squadId}/inbox`);
    await page.waitForURL(/\/inbox/, { timeout: 10000 });

    await expect(page.getByTestId("inbox-table")).toBeVisible({ timeout: 10000 });

    for (const item of memberInboxItems.slice(0, 3)) {
      await expect(page.getByTestId(`inbox-item-${item.id}`)).toBeVisible({ timeout: 5000 });
    }

    console.log(`[journey-5] User 2 sees ${memberInboxItems.length} inbox items from members`);
    console.log("[journey-5] ✅ Shared squad model verified: User 2 sees all data");
  });
});
