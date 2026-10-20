import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MobileSheet } from "@/features/navigation/mobile-sheet";

// The sheet is verified by how it can be dismissed and how it behaves when
// closed: it is a modal dialog while open, and inert while shut. The panel's
// contents are stubbed — this is about the chrome, not the navigation tree.
vi.mock("@/features/navigation/sidebar-content", () => ({
  SidebarContent: ({ onMobileClose }: { onMobileClose?: () => void }) => (
    <button type="button" onClick={onMobileClose}>
      Close from inside
    </button>
  ),
}));

function renderSheet(open = true) {
  const onClose = vi.fn();
  render(<MobileSheet open={open} onClose={onClose} hasTopBar />);
  return { onClose };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("MobileSheet", () => {
  it("is a modal dialog named as the main navigation", () => {
    renderSheet();

    const dialog = screen.getByRole("dialog", { name: "Main navigation" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAttribute("data-state", "expanded");
    expect(dialog).not.toHaveAttribute("inert");
  });

  it("anchors to the bottom edge instead of the side", () => {
    renderSheet();

    const dialog = screen.getByRole("dialog", { name: "Main navigation" });
    // A bottom sheet is positioned by its bottom edge, and its rounded corners
    // are on top: that is the whole shape of the change.
    expect(dialog.className).toContain("bottom-0");
    expect(dialog.className).toContain("inset-x-0");
    expect(dialog.className).toContain("rounded-t-");
    expect(dialog.className).not.toContain("top-0");
    expect(dialog.className).not.toContain("w-[86vw]");
  });

  it("closes on Escape", () => {
    const { onClose } = renderSheet();

    fireEvent.keyDown(screen.getByRole("dialog", { name: "Main navigation" }), {
      key: "Escape",
    });

    expect(onClose).toHaveBeenCalledOnce();
  });

  it("closes when the page behind it is tapped", () => {
    const { onClose } = renderSheet();

    fireEvent.click(
      screen.getByRole("button", { name: "Close sidebar overlay" }),
    );

    expect(onClose).toHaveBeenCalledOnce();
  });

  it("closes on a pointer press, so a tap dismisses immediately", () => {
    const { onClose } = renderSheet();

    fireEvent.pointerDown(
      screen.getByRole("button", { name: "Close sidebar overlay" }),
    );

    expect(onClose).toHaveBeenCalledOnce();
  });

  it("ignores a pointer-derived click, which can be the tail of the tap that opened it", () => {
    const { onClose } = renderSheet();

    // A real pointer tap produces a click with `detail >= 1`. Acting on it is
    // what made the opener look dead on a phone: the sheet opened on press and
    // the same gesture's click closed it again.
    fireEvent.click(
      screen.getByRole("button", { name: "Close sidebar overlay" }),
      {
        detail: 1,
      },
    );

    expect(onClose).not.toHaveBeenCalled();
  });

  it("closes from the navigation's own close control", () => {
    const { onClose } = renderSheet();

    fireEvent.click(screen.getByRole("button", { name: "Close from inside" }));

    expect(onClose).toHaveBeenCalledOnce();
  });

  it("is inert while closed so nothing behind it takes focus", () => {
    renderSheet(false);

    const dialog = document.querySelector('[role="dialog"]');
    expect(dialog).toHaveAttribute("aria-hidden", "true");
    expect(dialog).toHaveAttribute("data-state", "collapsed");
  });
});
