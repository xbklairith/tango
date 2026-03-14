import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { FormDialog } from "./form-dialog";

function renderDialog(overrides: Partial<Parameters<typeof FormDialog>[0]> = {}) {
  const defaults = {
    open: true,
    onOpenChange: vi.fn(),
    title: "Test Dialog",
    isPending: false,
    onSubmit: vi.fn(),
    children: <input data-testid="child-input" />,
    ...overrides,
  };
  return { ...render(<FormDialog {...defaults} />), ...defaults };
}

describe("FormDialog", () => {
  it("renders children when open=true", () => {
    renderDialog({ open: true });
    expect(screen.getByTestId("child-input")).toBeInTheDocument();
  });

  it("submit button shows 'Save' by default", () => {
    renderDialog();
    expect(screen.getByRole("button", { name: /save/i })).toBeInTheDocument();
  });

  it("submitLabel prop overrides button text", () => {
    renderDialog({ submitLabel: "Create" });
    expect(screen.getByRole("button", { name: /create/i })).toBeInTheDocument();
  });

  it("submit button is disabled when isPending=true", () => {
    renderDialog({ isPending: true });
    expect(screen.getByRole("button", { name: /save/i })).toBeDisabled();
  });

  it("calls onSubmit when form is submitted", async () => {
    const user = userEvent.setup();
    const { onSubmit } = renderDialog();
    await user.click(screen.getByRole("button", { name: /save/i }));
    expect(onSubmit).toHaveBeenCalled();
  });

  it("title prop appears as dialog title content", () => {
    renderDialog({ title: "My Form" });
    expect(screen.getByText("My Form")).toBeInTheDocument();
  });

  it("description appears when provided", () => {
    renderDialog({ description: "Fill in the details" });
    expect(screen.getByText("Fill in the details")).toBeInTheDocument();
  });

  it("fieldset is disabled when isPending=true", () => {
    renderDialog({ isPending: true });
    const fieldset = screen.getByTestId("child-input").closest("fieldset");
    expect(fieldset).toBeDisabled();
  });
});
