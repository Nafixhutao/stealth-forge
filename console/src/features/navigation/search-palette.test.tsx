import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SearchPalette } from "@/features/navigation/search-palette";

// The palette's contract is that every entry does something. It used to list
// fifteen destinations with no route behind them, so choosing one closed the
// palette and changed nothing; this pins the entries that remain and the two
// actions the palette performs itself.
const logoutMutate = vi.fn();
const replace = vi.fn();

vi.mock("@/api/mutations/auth", () => ({
  useLogout: () => ({ mutate: logoutMutate, isPending: false }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace, push: vi.fn() }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn() } }));

function renderPalette() {
  const onClose = vi.fn();
  const onSelect = vi.fn();
  render(<SearchPalette open onClose={onClose} onSelect={onSelect} />);
  return { onClose, onSelect };
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SearchPalette", () => {
  it("offers only entries that lead somewhere", () => {
    renderPalette();

    expect(
      screen.getByRole("button", { name: /Go to Projects/ }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Status page/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Log out/ })).toBeInTheDocument();

    for (const dead of [
      "Go to Repositories",
      "Go to Agents",
      "Go to Dashboard",
      "Go to Organization Settings",
      "Go to Billing",
      "Go to Profile",
      "Go to Documentation",
      "Contact Support",
      "Keyboard Shortcuts",
      "Release Notes",
      "API Reference",
      "Report a Problem",
    ]) {
      expect(screen.queryByRole("button", { name: dead })).toBeNull();
    }
  });

  it("hands a real destination to the sidebar", () => {
    const { onSelect } = renderPalette();

    fireEvent.click(screen.getByRole("button", { name: /Go to Projects/ }));

    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ navigateTo: "Projects" }),
    );
  });

  it("logs out without involving the sidebar", () => {
    const { onClose, onSelect } = renderPalette();

    fireEvent.click(screen.getByRole("button", { name: /Log out/ }));

    expect(onClose).toHaveBeenCalled();
    expect(logoutMutate).toHaveBeenCalled();
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("filters to what matches the query", () => {
    renderPalette();

    fireEvent.change(screen.getByLabelText("Search commands"), {
      target: { value: "log" },
    });

    expect(screen.getByRole("button", { name: /Log out/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Go to Projects/ })).toBeNull();
  });

  it("says so when nothing matches instead of showing a dead row", () => {
    renderPalette();

    fireEvent.change(screen.getByLabelText("Search commands"), {
      target: { value: "zzzz" },
    });

    expect(screen.getByText("No results found.")).toBeInTheDocument();
  });
});
