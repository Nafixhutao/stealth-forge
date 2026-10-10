import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { HttpStatusBadge, StatusBadge } from "@/components/ui/badge";

describe("StatusBadge", () => {
  it("renders the status label through the shared AnimatedBadge", () => {
    render(<StatusBadge status="pending" />);

    const label = screen.getByText("Pending");
    expect(label).toHaveAttribute("data-badge-label");
    // The badge is the shared pill primitive, not a bespoke span.
    expect(label.closest("span.rounded-full")).not.toBeNull();
  });

  it("maps a successful status onto the success tone", () => {
    render(<StatusBadge status="succeeded" />);

    const label = screen.getByText("Succeeded");
    const badge = label.closest("span.rounded-full");
    expect(badge?.className).toContain("--projects-accent");
  });

  it("renders an unknown status without crashing", () => {
    render(<StatusBadge status={null} />);
    expect(screen.getByText("Unknown")).toBeVisible();
  });
});

describe("HttpStatusBadge", () => {
  it("keeps the HTTP code visible while applying semantic status colors", () => {
    render(
      <>
        <HttpStatusBadge status={200} />
        <HttpStatusBadge status={404} />
        <HttpStatusBadge status={500} />
      </>,
    );

    expect(screen.getByText("200")).toHaveClass("text-emerald-200");
    expect(screen.getByText("404")).toHaveClass("text-amber-200");
    expect(screen.getByText("500")).toHaveClass("text-rose-200");
  });
});
