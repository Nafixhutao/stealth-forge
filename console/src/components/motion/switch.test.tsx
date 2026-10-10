import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Switch } from "@/components/motion/switch";

// Switch is the standard toggle primitive. These cases pin its accessible
// contract: a real switch role, the checked state, and change callbacks.
describe("Switch", () => {
  it("exposes a switch role and reflects the checked state", () => {
    render(<Switch checked onCheckedChange={() => {}} ariaLabel="Feature" />);

    const control = screen.getByRole("switch", { name: "Feature" });
    expect(control).toHaveAttribute("aria-checked", "true");
    expect(control).toHaveAttribute("data-state", "checked");
  });

  it("reports the next value when toggled", () => {
    const onChange = vi.fn();
    render(
      <Switch checked={false} onCheckedChange={onChange} ariaLabel="Feature" />,
    );

    fireEvent.click(screen.getByRole("switch", { name: "Feature" }));
    expect(onChange).toHaveBeenCalledWith(true);
  });

  it("does not fire when disabled", () => {
    const onChange = vi.fn();
    render(
      <Switch
        checked={false}
        onCheckedChange={onChange}
        disabled
        ariaLabel="Feature"
      />,
    );

    const control = screen.getByRole("switch", { name: "Feature" });
    fireEvent.click(control);
    expect(onChange).not.toHaveBeenCalled();
    expect(control).toBeDisabled();
  });

  it("renders an associated label when provided", () => {
    render(<Switch checked onCheckedChange={() => {}} label="Use TLS" />);

    expect(screen.getByText("Use TLS")).toBeVisible();
    expect(screen.getByRole("switch", { name: "Use TLS" })).toBeVisible();
  });
});
