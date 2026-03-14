import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { Textarea } from "./textarea";

describe("Textarea", () => {
  it("renders a <textarea> element", () => {
    render(<Textarea data-testid="ta" />);
    const el = screen.getByTestId("ta");
    expect(el.tagName).toBe("TEXTAREA");
  });

  it("applies custom className alongside base classes", () => {
    render(<Textarea data-testid="ta" className="my-class" />);
    const el = screen.getByTestId("ta");
    expect(el.className).toContain("my-class");
    expect(el.className).toContain("rounded");
  });

  it("forwards native textarea props", () => {
    render(<Textarea placeholder="Enter text" disabled />);
    const el = screen.getByPlaceholderText("Enter text");
    expect(el).toBeDisabled();
  });

  it("defaults to rows=3", () => {
    render(<Textarea data-testid="ta" />);
    const el = screen.getByTestId("ta") as HTMLTextAreaElement;
    expect(el.rows).toBe(3);
  });

  it("allows overriding rows", () => {
    render(<Textarea data-testid="ta" rows={5} />);
    const el = screen.getByTestId("ta") as HTMLTextAreaElement;
    expect(el.rows).toBe(5);
  });
});
