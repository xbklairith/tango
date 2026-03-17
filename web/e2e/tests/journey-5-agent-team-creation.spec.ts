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
    test.setTimeout(600_000);

    const email = `j5-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

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

    // Verify we're on the conversation page
    await page.waitForURL(/\/conversations\//, { timeout: 10000 });
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
  });
});
