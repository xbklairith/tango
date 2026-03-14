import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { PaginationControls } from "./pagination-controls";

describe("PaginationControls", () => {
  it("returns null when total <= limit", () => {
    const { container } = render(
      <PaginationControls total={20} offset={0} limit={20} onPageChange={vi.fn()} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it('renders range text "1–20 of 47" for offset=0', () => {
    render(<PaginationControls total={47} offset={0} limit={20} onPageChange={vi.fn()} />);
    expect(screen.getByText("1–20 of 47")).toBeInTheDocument();
  });

  it('renders range text "21–40 of 47" for offset=20', () => {
    render(<PaginationControls total={47} offset={20} limit={20} onPageChange={vi.fn()} />);
    expect(screen.getByText("21–40 of 47")).toBeInTheDocument();
  });

  it('renders range text "41–47 of 47" for offset=40', () => {
    render(<PaginationControls total={47} offset={40} limit={20} onPageChange={vi.fn()} />);
    expect(screen.getByText("41–47 of 47")).toBeInTheDocument();
  });

  it("Previous button is disabled when offset=0", () => {
    render(<PaginationControls total={47} offset={0} limit={20} onPageChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: /previous/i })).toBeDisabled();
  });

  it("Previous button is enabled when offset > 0", () => {
    render(<PaginationControls total={47} offset={20} limit={20} onPageChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: /previous/i })).not.toBeDisabled();
  });

  it("Next button is disabled when offset + limit >= total", () => {
    render(<PaginationControls total={47} offset={40} limit={20} onPageChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: /next/i })).toBeDisabled();
  });

  it("Next button is enabled when offset + limit < total", () => {
    render(<PaginationControls total={47} offset={0} limit={20} onPageChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: /next/i })).not.toBeDisabled();
  });

  it("clicking Next calls onPageChange(offset + limit)", async () => {
    const user = userEvent.setup();
    const onPageChange = vi.fn();
    render(<PaginationControls total={47} offset={0} limit={20} onPageChange={onPageChange} />);
    await user.click(screen.getByRole("button", { name: /next/i }));
    expect(onPageChange).toHaveBeenCalledWith(20);
  });

  it("clicking Previous calls onPageChange(offset - limit) clamped to 0", async () => {
    const user = userEvent.setup();
    const onPageChange = vi.fn();
    render(<PaginationControls total={47} offset={20} limit={20} onPageChange={onPageChange} />);
    await user.click(screen.getByRole("button", { name: /previous/i }));
    expect(onPageChange).toHaveBeenCalledWith(0);
  });
});
