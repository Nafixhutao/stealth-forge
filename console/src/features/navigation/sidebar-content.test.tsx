import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SidebarContent } from "@/features/navigation/sidebar-content";

// The panel header carries the organization selector, the plan chip, and the
// panel's own controls; the rows below it are the reference navigation. The
// account/organization hooks are stubbed — this file is about the panel.
const useCurrentAccount = vi.fn();
const useOrganizationPlan = vi.fn();
const useOrganizations = vi.fn();
const useCurrentOrganizationId = vi.fn();

vi.mock("@/api/queries/account", () => ({
  useCurrentAccount: () => useCurrentAccount(),
}));

vi.mock("@/api/queries/organizations", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/api/queries/organizations")>();
  return {
    ...actual,
    useOrganizationPlan: (id: string | undefined) => useOrganizationPlan(id),
    useOrganizations: () => useOrganizations(),
  };
});

vi.mock("@/features/navigation/use-current-organization", () => ({
  useCurrentOrganizationId: () => useCurrentOrganizationId(),
}));

const nav = vi.hoisted(() => ({
  push: vi.fn(),
  pathname: "/user",
}));

vi.mock("next/navigation", () => ({
  usePathname: () => nav.pathname,
  useRouter: () => ({ push: nav.push, replace: vi.fn() }),
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

const ORGANIZATION_ID = "11111111-1111-1111-1111-111111111111";

function organizations(name = "Nafixhutao") {
  return {
    data: {
      pages: [
        {
          organizations: [
            {
              id: ORGANIZATION_ID,
              name,
              slug: "nafixhutao",
              created_at: "2026-01-01T00:00:00Z",
            },
          ],
          pagination: { limit: 50, next_cursor: null },
        },
      ],
      pageParams: [undefined],
    },
  };
}

function plan(planKey: string, projects = 1, limit = 10) {
  return {
    data: {
      plan: {
        organization_id: ORGANIZATION_ID,
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
  nav.pathname = "/user";
  useCurrentOrganizationId.mockReturnValue(ORGANIZATION_ID);
  useOrganizations.mockReturnValue(organizations());
  useOrganizationPlan.mockReturnValue(plan("free"));
  useCurrentAccount.mockReturnValue({
    data: {
      account: {
        id: "22222222-2222-2222-2222-222222222222",
        email: "nafiaku447@gmail.com",
        display_name: "Nafi",
        avatar_url: "",
        instance_role: "instance_admin",
      },
    },
  });
});

describe("SidebarContent header", () => {
  it("names the organization the surface belongs to", () => {
    render(<SidebarContent />);

    expect(screen.getByText("Nafixhutao")).toBeInTheDocument();
  });

  it("drops the generated '<name>'s organization' suffix", () => {
    useOrganizations.mockReturnValue(organizations("Aku Nafi's organization"));

    render(<SidebarContent />);

    expect(screen.getByText("Aku Nafi")).toBeInTheDocument();
    expect(
      screen.queryByText("Aku Nafi's organization"),
    ).not.toBeInTheDocument();
  });

  it("falls back to the account name while organizations are loading", () => {
    useOrganizations.mockReturnValue({ data: undefined });

    render(<SidebarContent />);

    expect(screen.getByText("Nafi")).toBeInTheDocument();
  });

  it("falls back to the email's local part when there is no display name", () => {
    useOrganizations.mockReturnValue({ data: undefined });
    useCurrentAccount.mockReturnValue({
      data: {
        account: {
          id: "2",
          email: "nafi@example.com",
          display_name: "   ",
          avatar_url: "",
          instance_role: null,
        },
      },
    });

    render(<SidebarContent />);

    expect(screen.getByText("nafi")).toBeInTheDocument();
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

    expect(useOrganizationPlan).toHaveBeenCalledWith(ORGANIZATION_ID);
  });

  it("reads the plan of the previewed organization when no scope is set", () => {
    useCurrentOrganizationId.mockReturnValue(null);

    render(<SidebarContent />);

    expect(useOrganizationPlan).toHaveBeenCalledWith(ORGANIZATION_ID);
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
    const header = screen.getByText("Nafixhutao").closest("header");
    expect(header?.querySelectorAll("[class*='rounded-[4px]']")).toHaveLength(
      0,
    );
  });

  it("opens the organization home from the selector", () => {
    nav.pathname = "/projects/11111111-1111-1111-1111-111111111111";
    nav.push.mockClear();

    render(<SidebarContent />);

    fireEvent.click(
      screen.getByRole("button", { name: "Switch organization" }),
    );

    expect(nav.push).toHaveBeenCalledWith("/user");
  });

  it("offers the collapse control on the desktop panel", () => {
    const onToggleCollapse = vi.fn();
    render(<SidebarContent onToggleCollapse={onToggleCollapse} />);

    fireEvent.click(screen.getByRole("button", { name: "Collapse sidebar" }));

    expect(onToggleCollapse).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "Close sidebar" })).toBeNull();
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

    const name = screen.getByText("Nafixhutao");
    expect(name.className).toContain("truncate");
    expect(name.className).toContain("shrink");
    expect(name).toHaveAttribute("title", "Nafixhutao");
  });
});

describe("SidebarContent navigation", () => {
  it("draws the reference rows in order", () => {
    render(<SidebarContent />);

    const labels = screen
      .getAllByRole("button")
      .map((button) => button.textContent?.trim());

    const expected = [
      "Triage",
      "Reviews",
      "Agent",
      "Security",
      "Analytics",
      "Automations",
      "Slack Bot",
      "Scopes",
      "Organization settings",
    ];
    for (const label of expected) {
      expect(labels).toContain(label);
    }
    // Order matters: the reference is a fixed list, not an alphabetical menu.
    const positions = expected.map((label) => labels.indexOf(label));
    expect([...positions].sort((a, b) => a - b)).toEqual(positions);
  });

  it("keeps the old Projects row out of the reference navigation", () => {
    render(<SidebarContent />);

    expect(screen.queryByRole("button", { name: "Projects" })).toBeNull();
  });

  it("marks Triage as the current row", () => {
    render(<SidebarContent />);

    expect(screen.getByRole("button", { name: "Triage" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("puts the gift accent on Agent only", () => {
    const { container } = render(<SidebarContent />);

    const coding = screen.getByRole("button", { name: "Agent" });
    const gifts = (root: HTMLElement | Element) =>
      [...root.querySelectorAll("svg")].filter((svg) =>
        svg.getAttribute("class")?.includes("FFA057"),
      );

    expect(gifts(coding)).toHaveLength(1);
    expect(gifts(container)).toHaveLength(1);
  });

  it("keeps the search shortcut hint", () => {
    render(<SidebarContent />);

    expect(
      screen.getByRole("button", { name: /Search\s*Ctrl K/ }),
    ).toBeInTheDocument();
  });

  it("opens the Agent surface from the Agent row", () => {
    render(<SidebarContent />);

    fireEvent.click(screen.getByRole("button", { name: "Agent" }));

    expect(nav.push).toHaveBeenCalledWith("/agent");
  });

  it("marks Agent as current on the agent route, not Triage", () => {
    nav.pathname = "/agent";

    render(<SidebarContent />);

    expect(
      screen.getByRole("button", { name: "Agent" }),
    ).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "Triage" })).not.toHaveAttribute(
      "aria-current",
    );
  });
});
