import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SidebarContent } from "@/features/navigation/sidebar-content";

// The panel header carries the account identity and the panel's own controls.
// It used to also carry a plan chip that always read "Pro Plus" and a switcher
// glyph with nothing behind it; both are asserted absent so fabricated data
// cannot creep back in. The nav rows are stubbed: this file is about the header.
const useCurrentAccount = vi.fn();
const useOrganizationPlan = vi.fn();
const useCurrentOrganizationId = vi.fn();

vi.mock("@/api/queries/account", () => ({
  useCurrentAccount: () => useCurrentAccount(),
}));

vi.mock("@/api/queries/organizations", () => ({
  useOrganizationPlan: (id: string | undefined) => useOrganizationPlan(id),
}));

vi.mock("@/features/navigation/use-current-organization", () => ({
  useCurrentOrganizationId: () => useCurrentOrganizationId(),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => "/user",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
}));

vi.mock("@/features/navigation/profile-menu", () => ({
  BottomProfile: () => <footer data-testid="bottom-profile" />,
}));

vi.mock("@/features/navigation/sidebar-shared", async (importOriginal) => {
  const actual =
    await importOriginal<
      typeof import("@/features/navigation/sidebar-shared")
    >();
  return {
    ...actual,
    // The rows animate in with a stagger; jsdom has no layout, and this file is
    // not about them.
    NavIcon: () => null,
  };
});

function plan(planKey: string, projects = 1, limit = 10) {
  return {
    data: {
      plan: {
        organization_id: "11111111-1111-1111-1111-111111111111",
        plan_key: planKey,
        status: "active",
        limits: { projects: limit },
        usage: { projects },
      },
    },
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  useCurrentOrganizationId.mockReturnValue(
    "11111111-1111-1111-1111-111111111111",
  );
  useOrganizationPlan.mockReturnValue(plan("free"));
  useCurrentAccount.mockReturnValue({
    data: {
      account: {
        id: "22222222-2222-2222-2222-222222222222",
        email: "aulia@example.com",
        display_name: "Aulia Rahma Putri",
        avatar_url: "",
        instance_role: null,
      },
    },
  });
});

describe("SidebarContent header", () => {
  it("shows the signed-in account's name", () => {
    render(<SidebarContent />);

    expect(screen.getByText("Aulia Rahma Putri")).toBeInTheDocument();
  });

  it("falls back to the email's local part when there is no display name", () => {
    useCurrentAccount.mockReturnValue({
      data: {
        account: {
          id: "2",
          email: "aulia@example.com",
          display_name: "   ",
          avatar_url: "",
          instance_role: null,
        },
      },
    });

    render(<SidebarContent />);

    expect(screen.getByText("aulia")).toBeInTheDocument();
  });

  it("shows the organization's real plan, not a fixed label", () => {
    useOrganizationPlan.mockReturnValue(plan("pro", 3, 10));

    render(<SidebarContent />);

    expect(screen.getByText("Pro")).toBeInTheDocument();
    expect(screen.queryByText("Pro Plus")).toBeNull();
    // The chip explains itself with the real usage numbers.
    expect(screen.getByTitle("3 of 10 projects used")).toBeInTheDocument();
  });

  it("reads the plan for the organization the surface belongs to", () => {
    render(<SidebarContent />);

    expect(useOrganizationPlan).toHaveBeenCalledWith(
      "11111111-1111-1111-1111-111111111111",
    );
  });

  it("shows a free organization as Free", () => {
    useOrganizationPlan.mockReturnValue(plan("free", 1, 3));

    render(<SidebarContent />);

    expect(screen.getByText("Free")).toBeInTheDocument();
  });

  it("renders no chip until the plan is known", () => {
    useOrganizationPlan.mockReturnValue({ data: undefined });

    render(<SidebarContent />);

    // No placeholder that always says the same word: an empty slot is honest.
    const header = screen.getByText("Aulia Rahma Putri").closest("header");
    expect(header?.querySelectorAll("[class*='rounded-[5px]']")).toHaveLength(0);
  });

  it("does not render a switcher glyph with no action behind it", () => {
    render(<SidebarContent />);

    const header = screen.getByText("Aulia Rahma Putri").closest("header");
    expect(header?.querySelectorAll("svg")).toHaveLength(0);
  });

  it("offers the collapse control on the desktop panel", () => {
    const onToggleCollapse = vi.fn();
    render(<SidebarContent onToggleCollapse={onToggleCollapse} />);

    fireEvent.click(screen.getByRole("button", { name: "Collapse sidebar" }));

    expect(onToggleCollapse).toHaveBeenCalledOnce();
    expect(
      screen.queryByRole("button", { name: "Close sidebar" }),
    ).toBeNull();
  });

  it("offers the close control in the mobile sheet", () => {
    const onMobileClose = vi.fn();
    render(<SidebarContent onMobileClose={onMobileClose} />);

    fireEvent.click(screen.getByRole("button", { name: "Close sidebar" }));

    expect(onMobileClose).toHaveBeenCalledOnce();
    expect(
      screen.queryByRole("button", { name: "Collapse sidebar" }),
    ).toBeNull();
  });

  it("gives the name the room the row actually has", () => {
    render(<SidebarContent />);

    // The name is a truncating flex child, not a fixed 220px slot that cut
    // "Aulia Rahma Putri" short while the row still had empty space.
    const name = screen.getByText("Aulia Rahma Putri");
    expect(name.className).toContain("truncate");
    expect(name.className).toContain("flex-1");
    expect(name).toHaveAttribute("title", "Aulia Rahma Putri");
  });
});
