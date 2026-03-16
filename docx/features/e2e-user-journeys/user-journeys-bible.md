# Ari User Journeys — Behavioral Specification

> This document defines the expected user experience for four real-world team archetypes using Ari.
> Each journey describes **what the user does**, **what they see**, and **what must be true** at every step.
> These journeys serve as the source of truth for E2E test implementation.

---

## Principles

- Every journey starts from **zero state** — fresh user, no existing data
- The user interacts primarily through **the UI**, not the API
- API seeding is used only to simulate **agent-side behavior** (agents posting inbox items, cost events) that a human user cannot trigger through the UI
- Assertions focus on **what the user sees**, not internal state
- Each journey tells a **complete story** with a clear beginning, middle, and outcome

---

## Journey 1: Software Development Squad

### "Ship a Feature Sprint"

**Who**: Sarah, an engineering manager. She's setting up Ari for the first time to manage her team of AI coding agents.

**Goal**: Create a squad, onboard agents in a hierarchy, organize work into issues and a project, have a conversation with an agent about a technical decision, and see everything reflected on the dashboard.

---

#### Act 1 — First Login & Squad Creation

**Sarah registers and logs in for the first time.**

1. She navigates to `/login` and sees the login form with Email and Password fields.
2. She enters her credentials and clicks "Sign in".
3. She lands on the Dashboard. Because she has no squads yet, she sees a welcome empty state with a "Create Squad" button.

**She creates her first squad.**

4. She clicks "Create Squad". A dialog opens showing "Step 1 of 2".
5. She fills in:
   - Name: "Frontend Team"
   - Issue Prefix: "FE"
   - Description: "Frontend engineering squad"
6. She clicks "Next". The dialog advances to Step 2, asking for captain details.
7. She fills in:
   - Captain Name: "Orchestrator"
   - Captain Short Name: "orchestrator"
8. She clicks "Create Squad". The dialog closes.

**What must be true:**
- The dashboard now shows "Frontend Team" as the active squad
- The stat cards show: Active Agents 0, In Progress 0, Open Issues 0, Projects 0
- The captain "Orchestrator" exists but hasn't been counted as "active" yet (it starts as pending_approval or auto-activated depending on squad settings)

---

#### Act 2 — Building the Agent Team

**Sarah navigates to the Agents page to see her captain and build a team.**

9. She clicks "Agents" in the sidebar.
10. She sees a table with one row: "Orchestrator" with role "Captain".

**She creates a lead agent.**

11. She clicks "Create Agent".
12. She fills in:
    - Name: "Code Reviewer"
    - URL Key: "code-reviewer"
    - Role: "Lead"
    - Reports To: "Orchestrator"
13. She submits. The agent list now shows 2 agents.

**She creates a member agent.**

14. She clicks "Create Agent" again.
15. She fills in:
    - Name: "Junior Coder"
    - URL Key: "junior-coder"
    - Role: "Member"
    - Reports To: "Code Reviewer"
16. She submits. The agent list now shows 3 agents.

**What must be true:**
- The hierarchy is: Orchestrator (Captain) → Code Reviewer (Lead) → Junior Coder (Member)
- Each agent row shows the correct role badge
- Each agent row shows a status badge

**She approves the agents to make them active.**

17. She clicks on "Code Reviewer" in the table. The agent detail page opens.
18. She sees the agent name, status badge showing "Pending Approval", and an "Approve" button.
19. She clicks "Approve". The status badge changes to "Active".
20. She navigates back to the agent list, clicks "Junior Coder", and approves it the same way.

**What must be true:**
- Both agents now show "Active" status
- The dashboard's "Active Agents" count reflects the activated agents

---

#### Act 3 — Organizing Work

**Sarah creates issues and assigns them to agents.**

21. She navigates to Issues via the sidebar.
22. She clicks "Create Issue" and fills in:
    - Title: "Design new auth flow"
    - Type: Task
    - Priority: Critical
    - Assignee: Code Reviewer
23. She submits. The issue "FE-1" appears in the list with "Critical" priority badge.

24. She creates another issue:
    - Title: "Implement token refresh"
    - Type: Task
    - Priority: High
    - Assignee: Junior Coder
25. She submits. "FE-2" appears in the list.

**What must be true:**
- Issue identifiers follow the format "{prefix}-{counter}": FE-1, FE-2
- Each issue shows its priority badge and assigned agent
- The dashboard "Open Issues" count is now 2

---

#### Act 4 — Working an Issue Through Its Lifecycle

**Sarah clicks on FE-1 to work on it.**

26. She sees the issue detail page with:
    - Identifier "FE-1" in monospace
    - Title "Design new auth flow"
    - Status card showing "Backlog"
    - Priority card showing "Critical"
    - Type card showing "Task"
27. Below the cards, she sees transition buttons for valid next statuses.

**She moves the issue through statuses.**

28. She clicks "Todo". The status card updates to "Todo". The transition buttons change to show the next valid transitions (In Progress, Backlog, Blocked, Cancelled).
29. She clicks "In Progress". Status updates. Buttons change again (Done, Blocked, Cancelled).

**She adds a comment.**

30. She scrolls to the Comments section. She sees "No comments yet."
31. She types "Starting the design work now" in the comment textarea.
32. She clicks "Add Comment".
33. The comment appears with her display name and a timestamp.

**She edits the issue.**

34. She clicks "Edit" in the header. The page switches to edit mode with Title, Description, and Priority fields.
35. She changes the title to "Design new auth flow (Revised)" and changes priority to "High".
36. She clicks "Save". The detail page updates with the new title and priority.

**What must be true:**
- Status transitions follow the valid state machine (backlog → todo → in_progress)
- Comments are append-only with correct attribution
- Edit mode preserves existing values and applies changes immediately

---

#### Act 5 — Conversation with an Agent

**Sarah wants to discuss a technical decision with the Code Reviewer agent.**

37. She navigates to Conversations via the sidebar.
38. She sees an empty state: "No conversations yet. Start one to chat with an agent."

39. She clicks "New Conversation". A dialog opens with fields:
    - Agent (dropdown of active agents)
    - Title
    - First Message (optional)
40. She selects "Code Reviewer", types title "Discuss API contract for auth", types first message "What's your recommendation for the token format?"
41. She clicks "Start".

42. She's redirected to the conversation page. She sees:
    - The conversation title "Discuss API contract for auth" in the header
    - The conversation identifier (e.g., "FE-3")
    - "Active" status
    - Her message displayed as a user bubble (right-aligned, with User icon)
    - A "Close" button in the header
    - A text input area at the bottom with "Type a message..." placeholder

**She sends another message.**

43. She types "Also, should we support refresh tokens?" and presses Enter.
44. A second user message bubble appears.

**She closes the conversation.**

45. She clicks the "Close" button in the header.
46. The status changes to "Closed". The input area is replaced with "This conversation is closed."

**What must be true:**
- Conversations are displayed as a chat interface with message bubbles
- User messages appear right-aligned with User icon
- Agent messages (if any) appear left-aligned with Bot icon
- Enter key sends messages (Shift+Enter for newline)
- Closed conversations disable the input area
- The conversation appears in the conversation list

---

#### Act 6 — Dashboard Verification

**Sarah returns to the dashboard to see the full picture.**

47. She clicks "Dashboard" in the sidebar.
48. She sees stat cards reflecting all her work:
    - Active Agents: reflects activated agents
    - In Progress: at least 1 (FE-1)
    - Open Issues: reflects total open issues
    - Projects: 0 (she didn't create one in this journey)

**What must be true:**
- Dashboard stats are accurate and up-to-date after all operations
- Activity feed shows recent entries

---

## Journey 2: Investment Research Team

### "Approval Gate & Inbox Triage"

**Who**: Marcus, a portfolio manager. He uses Ari to govern AI research agents that analyze markets and request permission before executing trades.

**Goal**: Review and triage inbox items — approve a trade request, answer a research question, dismiss a budget alert. Experience the full inbox workflow with filters.

---

#### Preconditions (API-seeded, simulating agent behavior)

- Squad "Alpha Fund Research" exists with prefix "AFR"
- Captain agent auto-created with squad
- Lead agent "Senior Analyst" (active)
- Member agent "Data Collector" (active)
- Goal "Q1 Research Pipeline" exists
- 2 issues exist: "Research NVDA earnings", "Compile macro indicators"

---

#### Act 1 — Dashboard Overview

1. Marcus logs in and sees the dashboard.
2. He sees "Alpha Fund Research" as his active squad.
3. Stat cards show agent count and open issues matching seeded data.

**What must be true:**
- Dashboard accurately reflects API-seeded data

---

#### Act 2 — Inbox Items Arrive (API-seeded)

Three inbox items are created via API (simulating agent-generated requests):

- **Approval** (critical urgency): "Approve NVDA position increase" — Senior Analyst requests to increase NVDA position by 5%
- **Question** (normal urgency): "Clarify macro outlook scope" — Data Collector asks whether to include emerging markets
- **Alert** (normal urgency): "Data Collector approaching budget limit" — Agent has used 78% of monthly budget

---

#### Act 3 — Inbox List & Visual Hierarchy

4. Marcus navigates to Inbox via the sidebar.
5. He sees 3 inbox items in a table with columns: urgency icon, Title, Category, Status, Created, Actions.

**What he sees for each item:**
- **Urgency icons**: Red triangle (AlertTriangle) for "critical", gray circle for "normal"
- **Category badges**:
  - "Approval" in purple (bg-purple-100 text-purple-800)
  - "Question" in blue (bg-blue-100 text-blue-800)
  - "Alert" in yellow (bg-yellow-100 text-yellow-800)
- **Status badges**: All show "Pending" in yellow
- **Action buttons**:
  - Approval item: Eye icon (Acknowledge) + CheckCircle link (Resolve)
  - Question item: Eye icon (Acknowledge) + CheckCircle link (Resolve)
  - Alert item: Eye icon (Acknowledge) + X icon (Dismiss)

**What must be true:**
- Critical items are visually distinct from normal items
- Category badges use correct colors
- Action buttons match category (alerts get Dismiss, others get Resolve link)

---

#### Act 4 — Filtering

6. Marcus selects Category filter: "Approval".
7. Only the NVDA approval item is visible.
8. He clicks "Clear Filters". All 3 items return.
9. He selects Urgency filter: "Critical".
10. Only the critical approval item is visible.
11. He clicks "Clear Filters".

**What must be true:**
- Filters update the list immediately
- "Clear Filters" button only appears when filters are active
- Clearing restores the full list

---

#### Act 5 — Acknowledge & Resolve the Approval

12. Marcus clicks the Eye icon (title="Acknowledge") on "Approve NVDA position increase".
13. The status badge on that row changes from "Pending" to "Acknowledged" (blue badge).
14. The Eye icon disappears (acknowledge is only available for pending items).

15. Marcus clicks the item title "Approve NVDA position increase" to go to the detail page.
16. He sees:
    - Breadcrumb: "Inbox / Approve NVDA position increase"
    - Title in header
    - Metadata grid: Category "Approval", Urgency "Critical", Status "Acknowledged", Type "trade_execution"
    - Body text: "Senior Analyst requests to increase NVDA position by 5%..."
    - Resolve section with three buttons: "Approve", "Reject", "Request Revision"
    - A "Response note" textarea
    - A "Submit Resolution" button (disabled until a resolution is selected)

17. Marcus clicks "Approve". The button switches from outline to default variant (highlighted).
18. He types in the response note: "Approved. Keep position under 8% of portfolio."
19. He clicks "Submit Resolution".

20. The resolve form disappears. A green banner appears:
    - "Resolved"
    - Resolution: "Approved"
    - Response note text visible
    - Resolved timestamp

**What must be true:**
- Resolution buttons toggle selection state (only one can be selected)
- Submit is disabled until a resolution is selected
- After resolve, the green banner replaces the resolve form
- Response note is preserved in the resolved view

---

#### Act 6 — Answer the Question

21. Marcus navigates back to Inbox (via breadcrumb or sidebar).
22. He clicks on "Clarify macro outlook scope" title.
23. He sees Category: "Question", resolve buttons: "Answered", "Dismissed".
24. He clicks "Answered" (highlights).
25. He types: "Yes, include emerging markets. Focus on BRICS nations."
26. He clicks "Submit Resolution".
27. Green resolved banner appears.

**What must be true:**
- Question category shows "Answered" and "Dismissed" as resolution options (not Approve/Reject)
- The flow is identical to approval resolution

---

#### Act 7 — Dismiss the Alert

28. Marcus navigates back to Inbox.
29. He finds "Data Collector approaching budget limit" (the alert).
30. He clicks the X icon (title="Dismiss") directly from the list row.
31. The item's status changes to "Resolved" in the list without navigating away.

**What must be true:**
- Alerts can be dismissed directly from the list (no need to visit detail page)
- Dismiss updates the status in-place
- The X button is only visible for alerts that are pending or acknowledged

---

#### Act 8 — Verify All Resolved

32. Marcus selects Status filter: "Resolved".
33. All 3 items appear with "Resolved" status badges (green).

**What must be true:**
- Filter correctly shows only resolved items
- All 3 items were successfully resolved through different flows

---

## Journey 3: Content Operations

### "Pipeline-Driven Content Production"

**Who**: Priya, a content operations lead. She manages AI agents that draft, review, and optimize content.

**Goal**: Set up a content pipeline with stages, create issues linked to a project, work issues through their full lifecycle (including reopening), and manage pipeline configuration.

---

#### Preconditions (API-seeded)

- Squad "Content Factory" exists with prefix "CF"
- Captain "Content Director" (active)
- Member "Writer Bot" (active)
- Member "SEO Analyst" (active)
- Project "Q1 Blog Series" exists

---

#### Act 1 — Pipeline Setup

1. Priya navigates to Pipelines via the sidebar.
2. She clicks "Create Pipeline" and fills in:
   - Name: "Content Pipeline"
   - Description: "Standard content production flow"
3. She submits. The pipeline appears in the list.

4. She clicks on "Content Pipeline" to go to the detail page.
5. She sees:
   - Pipeline name with "Active" badge (green)
   - Buttons: "Deactivate", "Edit", "Delete"
   - "Stages (0)" heading
   - An "Add Stage" form at the bottom with fields: Name, Position (auto-populated with next position), Assigned Agent dropdown

**She adds stages one by one:**

6. She types Name: "Drafting", leaves Position at auto-populated "1", selects Assigned Agent: "Writer Bot", clicks "Add".
7. A stage row appears in the table: Position 1, Name "Drafting", Assigned Agent "Writer Bot".

8. She types Name: "Editorial Review", Position auto-fills "2", selects "Content Director", clicks "Add".
9. She types Name: "SEO Optimization", Position auto-fills "3", selects "SEO Analyst", clicks "Add".
10. She types Name: "Published", Position auto-fills "4", leaves Assigned Agent as "None", clicks "Add".

**What must be true:**
- Stages appear in a table sorted by position
- Each stage shows position number, name, assigned agent name (or "-" if none)
- The "Add Stage" form resets after each successful add
- The position field auto-increments
- The heading updates: "Stages (4)"

---

#### Act 2 — Issue Creation with Project Linking

11. Priya navigates to Issues.
12. She clicks "Create Issue" and fills in:
    - Title: "Write blog post: AI in Healthcare"
    - Type: Task
    - Priority: High
    - Assignee: Writer Bot
    - Project: Q1 Blog Series
13. She submits. "CF-1" appears.

14. She creates two more:
    - "Write blog post: Future of Remote Work" — Priority: Medium, Assignee: Writer Bot, Project: Q1 Blog Series → "CF-2"
    - "Write blog post: Sustainable Tech" — Priority: Low, Assignee: Writer Bot, Project: Q1 Blog Series → "CF-3"

**What must be true:**
- All 3 issues have identifiers CF-1, CF-2, CF-3
- Each shows correct priority badge
- All are linked to "Q1 Blog Series" project

---

#### Act 3 — Issue Filtering

15. Priya selects Priority filter: "High".
16. Only CF-1 is visible.
17. She changes to Priority: "Medium".
18. Only CF-2 is visible.
19. She clears filters. All 3 issues return.

**What must be true:**
- Priority filter works correctly
- Switching filters replaces the previous filter
- Clear restores full list

---

#### Act 4 — Full Issue Lifecycle (Including Reopen)

20. Priya clicks on "CF-1" to go to the detail page.
21. She sees:
    - Status: "Backlog"
    - Linked project: "Q1 Blog Series" (as a link)
    - Transition buttons for valid next statuses

**She moves through the full lifecycle:**

22. Clicks "Todo" → Status becomes "Todo"
23. Clicks "In Progress" → Status becomes "In Progress"
24. Clicks "Done" → Status becomes "Done"

**She reopens the issue (editorial revision needed):**

25. From "Done" status, she sees "Todo" as a valid transition.
26. She clicks "Todo" → Status returns to "Todo"

**What must be true:**
- Status transitions follow the valid state machine at each step
- The available transition buttons change after each status change
- Reopening from "Done" back to "Todo" is possible (enabling revision cycles)

---

#### Act 5 — Issue Edit & Comments

27. Priya clicks "Edit".
28. The page switches to edit mode showing Title, Description, and Priority fields.
29. She changes the title to "Write blog post: AI in Healthcare (Revised)" and changes priority to "Critical".
30. She clicks "Save".
31. The detail page shows the updated title and priority.

32. She types a comment: "First draft completed, 2500 words" and clicks "Add Comment".
33. The comment appears with her name and timestamp.

**What must be true:**
- Edit mode pre-fills current values
- Save returns to view mode with updated data
- Comments appear immediately after adding

---

#### Act 6 — Pipeline Management

34. Priya navigates back to Pipelines, clicks "Content Pipeline".

**She toggles the pipeline active state:**

35. She clicks "Deactivate". The badge changes from "Active" (green) to "Inactive" (gray). The button text changes to "Activate".
36. She clicks "Activate". The badge returns to "Active".

**She deletes a stage:**

37. She clicks the trash icon on the "Published" stage row.
38. A browser confirmation dialog appears: "Are you sure you want to delete this stage?"
39. She confirms. The stage disappears. "Stages (3)" now shows.

**What must be true:**
- Toggle is immediate with visual feedback
- Stage deletion requires confirmation
- Stage count updates after deletion

---

## Journey 4: DevOps/SRE Incident Response

### "Real-Time Alert Triage & Incident Coordination"

**Who**: Alex, an SRE team lead. He uses Ari to coordinate AI agents that monitor infrastructure and respond to incidents.

**Goal**: Triage critical alerts quickly, make a scaling decision, create and manage an incident issue with timeline comments, coordinate via conversation, and manage agent status.

---

#### Preconditions (API-seeded)

- Squad "SRE On-Call" exists with prefix "SRE"
- Captain "Incident Commander" (active)
- Lead "Alert Triager" (active)
- Member "Runbook Executor" (active)

---

#### Act 1 — Alerts Arrive (API-seeded)

Three inbox items created via API (simulating monitoring agent behavior):

- **Alert** (critical): "Production database CPU at 95%"
- **Alert** (critical): "API latency spike detected — p99 > 5s"
- **Decision** (critical): "Auto-scale cluster beyond budget limit?"

---

#### Act 2 — Inbox Triage

1. Alex logs in and navigates to Inbox.
2. He sees 3 items. All have critical urgency (red triangle icons).
3. Category badges show: Alert (yellow) x2, Decision (orange) x1.

**He dismisses the lower-priority alert from the list:**

4. He finds "Production database CPU at 95%".
5. He clicks the X icon (title="Dismiss") directly in the row.
6. The item resolves in-place — status changes to "Resolved".

**He handles the second alert via detail page:**

7. He clicks on "API latency spike detected — p99 > 5s" title.
8. On the detail page, he clicks "Acknowledge" (top-right button, visible because status is "pending").
9. Status changes to "Acknowledged". The Acknowledge button disappears.
10. He scrolls to the resolve section. For alert category, he sees a "Dismiss" button (no Approve/Reject).
11. He clicks "Dismiss".
12. Green resolved banner appears.

**He resolves the scaling decision:**

13. He navigates back to Inbox.
14. He clicks on "Auto-scale cluster beyond budget limit?" title.
15. Category: "Decision". Resolve buttons show: "Answered", "Dismissed".
16. He clicks "Answered".
17. He types: "Approved temporary scale-up for 4 hours. Monitor costs closely."
18. He clicks "Submit Resolution".
19. Resolved successfully.

**What must be true:**
- Alerts can be dismissed from list (fast triage) OR from detail page
- Decisions use "Answered"/"Dismissed" resolutions
- All critical items show red triangle icons
- Triage feels fast — dismiss from list takes one click

---

#### Act 3 — Incident Issue Management

**Alex creates an incident issue.**

20. He navigates to Issues.
21. He clicks "Create Issue" and fills in:
    - Title: "INC-P1: API Latency Degradation"
    - Type: Task
    - Priority: Critical
22. He submits. "SRE-1" appears.

23. He clicks on "SRE-1".
24. He clicks "In Progress" (or "Todo" then "In Progress" depending on initial status).

**He adds timeline comments as an incident log:**

25. He types: "14:32 — Alert triggered. API p99 latency exceeded 5s threshold." → clicks "Add Comment".
26. Comment appears with his name and timestamp.
27. He types: "14:35 — Triager identified root cause: database connection pool exhaustion." → clicks "Add Comment".
28. Both comments are visible in chronological order.

**What must be true:**
- Issue comments serve as an incident timeline
- Comments appear in order with attribution
- Issue can be moved to "In Progress" to indicate active incident

---

#### Act 4 — Incident Coordination Conversation

29. Alex navigates to Conversations.
30. He clicks "New Conversation".
31. He selects agent: "Alert Triager", title: "Coordinate API latency incident response", first message: "What's the current connection pool status?"
32. He clicks "Start".

33. He's redirected to the conversation page. He sees:
    - Title in header
    - His first message as a user bubble (right-aligned)
    - A text input at the bottom

34. He types "Can you also check the replica lag?" and presses Enter.
35. Second user message bubble appears.

**He closes the conversation after the incident is resolved.**

36. He clicks "Close".
37. "This conversation is closed." replaces the input area.

**What must be true:**
- Conversation serves as a real-time coordination channel
- Messages send instantly on Enter
- Closed conversation clearly indicates no further messages can be sent

---

#### Act 5 — Agent Status Management

38. Alex navigates to Agents.
39. He sees 3 agents with status badges.
40. He clicks on "Alert Triager" to go to the detail page.

**He pauses the agent (end of incident, no longer needed):**

41. He sees the "Pause" button (visible because agent is active/idle).
42. He clicks "Pause". The status badge changes to "Paused". The button changes to "Resume".

**He resumes the agent:**

43. He clicks "Resume". Status returns to "Active". The button changes back to "Pause".

**He edits agent metadata:**

44. He clicks "Edit". The page switches to edit mode.
45. He changes Title to "Senior Alert Triager".
46. He clicks "Save". The detail page shows the updated title.

**What must be true:**
- Agent pause/resume provides immediate visual feedback
- Status transitions are reflected in the badge
- Edit mode pre-fills current values
- Save returns to view mode with updated data

---

#### Act 6 — Dashboard Verification

47. Alex navigates to Dashboard.
48. Stat cards reflect all created entities: agents, issues.

**What must be true:**
- Dashboard aggregates are accurate after all journey operations

---

## Appendix: Feature Coverage Matrix

| Feature | J1 Dev | J2 Research | J3 Content | J4 SRE |
|---------|--------|-------------|------------|--------|
| Register & Login | x | x | x | x |
| Squad Creation (UI wizard) | x | | | |
| Agent Creation (UI) | x | | | |
| Agent Approve | x | | | |
| Agent Pause/Resume | | | | x |
| Agent Edit | | | | x |
| Issue Create (UI) | x | | x | x |
| Issue Status Lifecycle | x | | x | x |
| Issue Reopen (Done→Todo) | | | x | |
| Issue Edit | x | | x | |
| Issue Comments | x | | x | x |
| Issue Filtering | | | x | |
| Conversations Start | x | | | x |
| Conversations Chat | x | | | x |
| Conversations Close | x | | | x |
| Pipeline Create | | | x | |
| Pipeline Stages | | | x | |
| Pipeline Toggle Active | | | x | |
| Pipeline Stage Delete | | | x | |
| Inbox List | | x | | x |
| Inbox Filters | | x | | |
| Inbox Acknowledge | | x | | x |
| Inbox Resolve (Approval) | | x | | |
| Inbox Resolve (Question) | | x | | |
| Inbox Resolve (Decision) | | | | x |
| Inbox Dismiss (Alert) | | x | | x |
| Dashboard Stats | x | x | | x |

---

## Appendix: API Seeding Requirements

Journeys 2–4 require API-seeded data to keep tests focused on specific UI workflows. Here is what each journey needs seeded before UI interaction begins:

### Journey 2 — Investment Research
- Register user + login via API → get cookies
- POST squad "Alpha Fund Research" (prefix AFR) → get squad ID + captain ID
- POST agent "Senior Analyst" (lead, parent=captain) → get agent ID
- POST agent "Data Collector" (member, parent=Senior Analyst) → get agent ID
- PATCH both agents status → "active"
- POST goal "Q1 Research Pipeline"
- POST 2 issues
- POST 3 inbox items (approval, question, alert) with relatedAgentId

### Journey 3 — Content Operations
- Register user + login via API → get cookies
- POST squad "Content Factory" (prefix CF) → get squad ID + captain ID
- POST agent "Writer Bot" (member, parent=captain) → get agent ID
- POST agent "SEO Analyst" (member, parent=captain) → get agent ID
- PATCH all agents status → "active"
- POST project "Q1 Blog Series"
- POST pipeline "Content Pipeline"

### Journey 4 — SRE Incident Response
- Register user + login via API → get cookies
- POST squad "SRE On-Call" (prefix SRE) → get squad ID + captain ID
- POST agent "Alert Triager" (lead, parent=captain) → get agent ID
- POST agent "Runbook Executor" (member, parent=Alert Triager) → get agent ID
- PATCH all agents status → "active"
- POST 3 inbox items (2 alerts critical, 1 decision critical)
