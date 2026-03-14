import { describe, it, expect } from "vitest";
import { humanize } from "./utils";

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
