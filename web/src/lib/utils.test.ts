import { describe, it, expect } from "vitest";
import { humanize, buildQueryString } from "./utils";

describe("humanize", () => {
  it('converts "in_progress" to "In progress"', () => {
    expect(humanize("in_progress")).toBe("In progress");
  });

  it('converts "pending_approval" to "Pending approval"', () => {
    expect(humanize("pending_approval")).toBe("Pending approval");
  });

  it('converts "done" to "Done"', () => {
    expect(humanize("done")).toBe("Done");
  });

  it("returns empty string for empty input", () => {
    expect(humanize("")).toBe("");
  });

  it('converts "backlog" to "Backlog"', () => {
    expect(humanize("backlog")).toBe("Backlog");
  });
});

describe("buildQueryString", () => {
  it("includes only defined filter values", () => {
    const qs = buildQueryString(
      { status: "todo", priority: undefined },
      { offset: 0, limit: 20 },
    );
    expect(qs).toContain("status=todo");
    expect(qs).not.toContain("priority");
    expect(qs).toContain("limit=20");
    expect(qs).not.toContain("offset");
  });

  it("includes offset when > 0", () => {
    const qs = buildQueryString({}, { offset: 20, limit: 20 });
    expect(qs).toContain("offset=20");
  });

  it("combines multiple filters", () => {
    const qs = buildQueryString(
      { status: "done", priority: "high", assigneeAgentId: "agent-1" },
      { offset: 0, limit: 20 },
    );
    expect(qs).toContain("status=done");
    expect(qs).toContain("priority=high");
    expect(qs).toContain("assigneeAgentId=agent-1");
  });
});
