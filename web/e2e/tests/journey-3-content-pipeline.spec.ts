import { test, expect, loginViaAPI, registerViaAPI } from "../fixtures/test-fixtures";

test.describe("Journey 3: Pipeline-Driven Content Production", () => {
  test("full pipeline content production lifecycle", async ({ page, apiContext }) => {
    // ── Unique user & password ──
    const email = `j3-${Date.now()}@e2e.test`;
    const password = "TestP@ss1234!";

    // ── API Seeding ──
    await registerViaAPI(apiContext, email, "Priya", password);
    const cookies = await loginViaAPI(apiContext, email, password);

    // Create squad
    const squadResp = await apiContext.post("/api/squads", {
      data: {
        name: "Content Factory",
        issuePrefix: "CF",
        captainName: "Content Director",
        captainShortName: "content-director",
      },
      headers: { Cookie: cookies },
    });
    const squad = await squadResp.json();

    // Get captain
    const agentsResp = await apiContext.get(`/api/agents?squadId=${squad.id}`, {
      headers: { Cookie: cookies },
    });
    const agentsList = await agentsResp.json();
    const captain = agentsList.find((a: { role: string }) => a.role === "captain");

    // Create agents
    const writerResp = await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "Writer Bot",
        shortName: "writer-bot",
        role: "lead",
        parentAgentId: captain.id,
      },
      headers: { Cookie: cookies },
    });
    expect(writerResp.ok()).toBeTruthy();
    const writer = await writerResp.json();

    const seoResp = await apiContext.post("/api/agents", {
      data: {
        squadId: squad.id,
        name: "SEO Analyst",
        shortName: "seo-analyst",
        role: "lead",
        parentAgentId: captain.id,
      },
      headers: { Cookie: cookies },
    });
    expect(seoResp.ok()).toBeTruthy();
    const seo = await seoResp.json();

    // Activate agents
    await apiContext.patch(`/api/agents/${captain.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });
    await apiContext.patch(`/api/agents/${writer.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });
    await apiContext.patch(`/api/agents/${seo.id}`, {
      data: { status: "active" },
      headers: { Cookie: cookies },
    });

    // Create project
    await apiContext.post(`/api/squads/${squad.id}/projects`, {
      data: { name: "Q1 Blog Series" },
      headers: { Cookie: cookies },
    });

    // ── Login via UI ──
    await new Promise((r) => setTimeout(r, 1100));
    await page.goto("/login");
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await page.waitForURL((url) => !url.pathname.includes("/login"), {
      timeout: 10000,
    });

    // ══════════════════════════════════════════════
    // Act 1 — Pipeline Setup
    // ══════════════════════════════════════════════

    // Navigate to Pipelines via sidebar
    await page.getByRole("link", { name: "Pipelines" }).click();
    await expect(page.getByText("Pipelines")).toBeVisible({ timeout: 10000 });

    // Click "Create Pipeline" → dialog opens
    await page.getByRole("button", { name: "Create Pipeline" }).click();
    await expect(page.getByRole("heading", { name: "Create Pipeline" })).toBeVisible();

    // Fill pipeline form
    await page.locator("#pipeline-name").fill("Content Pipeline");
    await page.locator("#pipeline-desc").fill("Standard content production flow");

    // Click Save inside dialog
    await page.getByRole("button", { name: "Save" }).click();

    // Pipeline appears in list
    await expect(page.getByRole("link", { name: "Content Pipeline" })).toBeVisible({ timeout: 10000 });

    // Click on "Content Pipeline" → detail page
    await page.getByRole("link", { name: "Content Pipeline" }).click();

    // Verify detail page
    await expect(page.getByRole("heading", { name: "Content Pipeline" })).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Active")).toBeVisible();
    await expect(page.getByRole("button", { name: "Deactivate" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Edit" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Delete" })).toBeVisible();
    await expect(page.getByText("Stages (0)")).toBeVisible();

    // Helper: add stage with proper position fill
    async function addStage(name: string, position: string, agentName?: string) {
      await page.locator("#stage-name").fill(name);
      // Must explicitly fill position to trigger React state change
      const posInput = page.locator("#stage-position");
      await posInput.fill(position);
      if (agentName) {
        await page.getByRole("combobox").click();
        await page.getByRole("option", { name: agentName }).click();
      }
      await page.getByRole("button", { name: "Add" }).click();
      await expect(page.locator("table").getByText(name)).toBeVisible({ timeout: 10000 });
    }

    await addStage("Drafting", "1", "Writer Bot");
    await addStage("Editorial Review", "2", "Content Director");
    await addStage("SEO Optimization", "3", "SEO Analyst");
    await addStage("Published", "4");

    // Verify 4 stages
    await expect(page.getByText("Stages (4)")).toBeVisible();

    // ══════════════════════════════════════════════
    // Act 2 — Issue Creation with Project
    // ══════════════════════════════════════════════

    // Navigate to Issues via sidebar
    await page.getByRole("link", { name: "Issues" }).click();
    await expect(page.locator("h2").filter({ hasText: "Issues" })).toBeVisible({ timeout: 10000 });

    // ── Issue 1: AI in Healthcare ──
    await page.getByRole("button", { name: "Create Issue" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();

    await page.locator("#issue-title").fill("Write blog post: AI in Healthcare");
    // Select Type: Task
    const typeGroup = page.locator(".space-y-1").filter({ hasText: "Type" });
    await typeGroup.getByRole("combobox").click();
    await page.getByRole("option", { name: "Task" }).click();
    // Select Priority: High
    const priorityGroup = page.locator(".space-y-1").filter({ hasText: "Priority" });
    await priorityGroup.getByRole("combobox").click();
    await page.getByRole("option", { name: "High" }).click();
    // Select Assignee: Writer Bot
    const assigneeGroup = page.locator(".space-y-1").filter({ hasText: "Assignee" });
    await assigneeGroup.getByRole("combobox").click();
    await page.getByRole("option", { name: "Writer Bot" }).click();
    // Select Project: Q1 Blog Series
    const projectGroup = page.locator(".space-y-1").filter({ hasText: "Project" });
    await projectGroup.getByRole("combobox").click();
    await page.getByRole("option", { name: "Q1 Blog Series" }).click();
    // Submit
    await page.getByRole("button", { name: "Save" }).click();

    // CF-1 appears
    await expect(page.getByText("CF-1")).toBeVisible({ timeout: 10000 });

    // ── Issue 2: Future of Remote Work ──
    await page.getByRole("button", { name: "Create Issue" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();

    await page.locator("#issue-title").fill("Write blog post: Future of Remote Work");
    const typeGroup2 = page.locator(".space-y-1").filter({ hasText: "Type" });
    await typeGroup2.getByRole("combobox").click();
    await page.getByRole("option", { name: "Task" }).click();
    const priorityGroup2 = page.locator(".space-y-1").filter({ hasText: "Priority" });
    await priorityGroup2.getByRole("combobox").click();
    await page.getByRole("option", { name: "Medium" }).click();
    const assigneeGroup2 = page.locator(".space-y-1").filter({ hasText: "Assignee" });
    await assigneeGroup2.getByRole("combobox").click();
    await page.getByRole("option", { name: "Writer Bot" }).click();
    const projectGroup2 = page.locator(".space-y-1").filter({ hasText: "Project" });
    await projectGroup2.getByRole("combobox").click();
    await page.getByRole("option", { name: "Q1 Blog Series" }).click();
    await page.getByRole("button", { name: "Save" }).click();

    await expect(page.getByText("CF-2")).toBeVisible({ timeout: 10000 });

    // ── Issue 3: Sustainable Tech ──
    await page.getByRole("button", { name: "Create Issue" }).click();
    await expect(page.getByRole("dialog")).toBeVisible();

    await page.locator("#issue-title").fill("Write blog post: Sustainable Tech");
    const typeGroup3 = page.locator(".space-y-1").filter({ hasText: "Type" });
    await typeGroup3.getByRole("combobox").click();
    await page.getByRole("option", { name: "Task" }).click();
    const priorityGroup3 = page.locator(".space-y-1").filter({ hasText: "Priority" });
    await priorityGroup3.getByRole("combobox").click();
    await page.getByRole("option", { name: "Low" }).click();
    const assigneeGroup3 = page.locator(".space-y-1").filter({ hasText: "Assignee" });
    await assigneeGroup3.getByRole("combobox").click();
    await page.getByRole("option", { name: "Writer Bot" }).click();
    const projectGroup3 = page.locator(".space-y-1").filter({ hasText: "Project" });
    await projectGroup3.getByRole("combobox").click();
    await page.getByRole("option", { name: "Q1 Blog Series" }).click();
    await page.getByRole("button", { name: "Save" }).click();

    await expect(page.getByText("CF-3")).toBeVisible({ timeout: 10000 });

    // ══════════════════════════════════════════════
    // Act 3 — Issue Filtering
    // ══════════════════════════════════════════════

    // Issue filters: Status (0), Priority (1), Assignee (2) — shadcn Select comboboxes
    const filterComboboxes = page.getByRole("combobox");

    // Filter by Priority: High (second combobox = index 1)
    await filterComboboxes.nth(1).click();
    await page.getByRole("option", { name: "High" }).click();

    // Only CF-1 visible
    await expect(page.getByText("CF-1")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("CF-2")).not.toBeVisible();
    await expect(page.getByText("CF-3")).not.toBeVisible();

    // Change to "Medium"
    await filterComboboxes.nth(1).click();
    await page.getByRole("option", { name: "Medium" }).click();

    // Only CF-2 visible
    await expect(page.getByText("CF-2")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("CF-1")).not.toBeVisible();
    await expect(page.getByText("CF-3")).not.toBeVisible();

    // Clear filters → all 3 return
    await page.getByRole("button", { name: "Clear Filters" }).click();
    await expect(page.getByText("CF-1")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("CF-2")).toBeVisible();
    await expect(page.getByText("CF-3")).toBeVisible();

    // ══════════════════════════════════════════════
    // Act 4 — Full Issue Lifecycle (Including Reopen)
    // ══════════════════════════════════════════════

    // Click CF-1 → detail page
    await page.getByRole("link", { name: "Write blog post: AI in Healthcare" }).click();
    await expect(page.getByText("CF-1")).toBeVisible({ timeout: 10000 });

    // Status card — the one in the grid with the status badge
    const statusCard = page.locator(".grid.gap-4 .rounded-lg.border").first();

    // Status: Backlog
    await expect(statusCard.getByText("Backlog")).toBeVisible();

    // Transition: Backlog → Todo
    await page.getByRole("button", { name: "Todo" }).click();
    await expect(statusCard.getByText("Todo")).toBeVisible({ timeout: 5000 });

    // Transition: Todo → In Progress
    await page.getByRole("button", { name: "In Progress" }).click();
    await expect(statusCard.getByText("In Progress")).toBeVisible({ timeout: 5000 });

    // Transition: In Progress → Done
    await page.getByRole("button", { name: "Done" }).click();
    await expect(statusCard.getByText("Done")).toBeVisible({ timeout: 5000 });

    // From Done, "Todo" should be available as a transition (reopen)
    await expect(page.getByRole("button", { name: "Todo" })).toBeVisible();

    // Reopen: Done → Todo
    await page.getByRole("button", { name: "Todo" }).click();
    await expect(statusCard.getByText("Todo")).toBeVisible({ timeout: 5000 });

    // ══════════════════════════════════════════════
    // Act 5 — Issue Edit & Comments
    // ══════════════════════════════════════════════

    // Click Edit → edit mode
    await page.getByRole("button", { name: "Edit" }).click();
    await expect(page.getByText("Edit Issue")).toBeVisible({ timeout: 5000 });

    // Change title
    await page.locator("#edit-title").clear();
    await page.locator("#edit-title").fill("Write blog post: AI in Healthcare (Revised)");

    // Change priority to Critical
    const editPriorityGroup = page.locator(".space-y-1").filter({ hasText: "Priority" });
    await editPriorityGroup.getByRole("combobox").click();
    await page.getByRole("option", { name: "Critical" }).click();

    // Click Save
    await page.getByRole("button", { name: "Save" }).click();

    // See updated title and priority
    await expect(page.getByText("Write blog post: AI in Healthcare (Revised)")).toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Critical")).toBeVisible();

    // Add a comment
    await page.getByPlaceholder("Add a comment...").fill("First draft completed, 2500 words");
    await page.getByRole("button", { name: "Add Comment" }).click();

    // Comment appears
    await expect(page.getByText("First draft completed, 2500 words")).toBeVisible({ timeout: 10000 });

    // ══════════════════════════════════════════════
    // Act 6 — Pipeline Management
    // ══════════════════════════════════════════════

    // Navigate to Pipelines, click "Content Pipeline"
    await page.getByRole("link", { name: "Pipelines" }).click();
    await expect(page.getByRole("link", { name: "Content Pipeline" })).toBeVisible({ timeout: 10000 });
    await page.getByRole("link", { name: "Content Pipeline" }).click();
    await expect(page.getByRole("heading", { name: "Content Pipeline" })).toBeVisible({ timeout: 10000 });

    // Deactivate → badge changes to "Inactive", button text changes to "Activate"
    await page.getByRole("button", { name: "Deactivate" }).click();
    await expect(page.getByText("Inactive")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: "Activate" })).toBeVisible();

    // Activate → badge returns to "Active"
    await page.getByRole("button", { name: "Activate" }).click();
    await expect(page.getByText("Active")).toBeVisible({ timeout: 5000 });
    await expect(page.getByRole("button", { name: "Deactivate" })).toBeVisible();

    // Delete the "Published" stage row
    // Accept the browser confirmation dialog
    page.on("dialog", async (dialog) => {
      await dialog.accept();
    });

    // Find the "Published" stage row and click the delete button (Trash2 icon)
    const publishedRow = page.getByRole("row").filter({ hasText: "Published" });
    await publishedRow.getByRole("button").click();

    // Stage disappears, count updates to 3
    await expect(page.getByRole("cell", { name: "Published" })).not.toBeVisible({ timeout: 10000 });
    await expect(page.getByText("Stages (3)")).toBeVisible();
  });
});
