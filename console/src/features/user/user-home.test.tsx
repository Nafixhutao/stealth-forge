import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UserHome } from "@/features/user/user-home";

// The view is verified through its own contract: what it shows for each data
// state, and what its controls do. The query and mutation hooks are stubbed so
// the assertions are about this component, not about the network.
const useOrganizations = vi.fn();
const useOrganizationProjects = vi.fn();
const useOrganizationPlan = vi.fn();
const useCurrentAccount = vi.fn();
const useProjectRequestCount = vi.fn();

vi.mock("@/api/queries/organizations", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/api/queries/organizations")>();
  return {
    // The page reads the flattened list through this helper; only the hooks are
    // stubbed, so the real helper keeps its behaviour.
    organizationItems: actual.organizationItems,
    useOrganizations: () => useOrganizations(),
    useOrganizationProjects: (id: string | undefined) =>
      useOrganizationProjects(id),
    useOrganizationPlan: (id: string | undefined) => useOrganizationPlan(id),
  };
});

vi.mock("@/api/queries/account", () => ({
  useCurrentAccount: () => useCurrentAccount(),
}));

// Each card asks for its own 30-day count; the stub keeps this file about the
// page, not about the per-card reads.
vi.mock("@/api/queries/projects", () => ({
  useProjectRequestCount: (projectId: string, enabled: boolean) =>
    useProjectRequestCount(projectId, enabled),
}));

vi.mock("@/api/mutations/projects", () => ({
  useCreateProject: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock("@/api/mutations/auth", () => ({
  useLogout: () => ({ mutate: vi.fn(), isPending: false }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
}));

type QueryStub = {
  data?: unknown;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  hasNextPage?: boolean;
  isFetchingNextPage?: boolean;
  fetchNextPage?: () => void;
};

function query(overrides: Partial<QueryStub> = {}): QueryStub {
  return {
    data: undefined,
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: vi.fn(),
    ...overrides,
  };
}

const acme = {
  id: "11111111-1111-1111-1111-111111111111",
  name: "Acme Inc",
  slug: "acme",
  created_at: "2026-03-12T09:30:00Z",
};

const beta = {
  id: "22222222-2222-2222-2222-222222222222",
  name: "Beta Labs",
  slug: "beta-labs",
  created_at: "2026-04-04T11:00:00Z",
};

/** An infinite read's shape, matching `useOrganizations`. */
function organizationsPage(
  organizations: unknown[],
  options: { nextCursor?: string | null } = {},
) {
  const { nextCursor = null } = options;
  return {
    pages: [
      {
        organizations,
        pagination: { limit: 50, next_cursor: nextCursor },
      },
    ],
  };
}

/** An infinite read's shape: one or more pages, each with its own cursor. */
function projectsPage(
  projects: unknown[],
  options: { canManage?: boolean; nextCursor?: string | null } = {},
) {
  const { canManage = true, nextCursor = null } = options;
  return {
    pages: [
      {
        projects,
        pagination: { limit: 50, next_cursor: nextCursor },
        can_manage: canManage,
      },
    ],
  };
}

function plan(planKey: string, used: number, limit: number) {
  return {
    plan: {
      organization_id: acme.id,
      plan_key: planKey,
      status: "active",
      current_period_start: "2026-04-01",
      current_period_end: "2026-05-01",
      limits: {
        projects: limit,
        members: 5,
        databases: 5,
        storage_buckets: 5,
        functions: 5,
        sites: 5,
        apps: 5,
      },
      usage: {
        projects: used,
        members: 1,
        databases: 0,
        storage_buckets: 0,
        functions: 0,
        sites: 0,
        apps: 0,
      },
    },
  };
}

const checkout = {
  id: "p-1",
  organization_id: acme.id,
  name: "checkout-api",
  created_at: "2026-04-04T11:00:00Z",
};

const inventory = {
  id: "p-2",
  organization_id: acme.id,
  name: "inventory-worker",
  created_at: "2026-05-20T14:45:00Z",
};

function renderHome() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <UserHome />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useProjectRequestCount.mockImplementation((projectId: string) =>
    projectId === "p-1" ? 12_400 : 0,
  );
  useCurrentAccount.mockReturnValue(
    query({
      data: {
        account: {
          id: "acc-1",
          email: "operator@example.com",
          display_name: "Operator",
          email_verified: true,
          instance_role: null,
          created_at: "2026-01-01T00:00:00Z",
        },
      },
    }),
  );
  useOrganizationProjects.mockReturnValue(query({ data: projectsPage([]) }));
  useOrganizationPlan.mockReturnValue(query({ data: plan("free", 0, 10) }));
});

describe("UserHome", () => {
  it("shows the selected organization and its real plan", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme, beta]) }),
    );

    renderHome();

    expect(
      screen.getByRole("heading", { name: "Acme Inc", level: 1 }),
    ).toBeInTheDocument();
    expect(screen.getByText("Free")).toBeInTheDocument();
    expect(useOrganizationProjects).toHaveBeenCalledWith(acme.id);
    expect(useOrganizationPlan).toHaveBeenCalledWith(acme.id);
  });

  it("renders the mobile top bar with navigation, search, and the account", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );

    renderHome();

    expect(
      screen.getByRole("button", { name: "Open navigation" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Search" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Account menu" }),
    ).toBeInTheDocument();
  });

  it("does not claim zero requests before the snapshot lands", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationProjects.mockReturnValue(
      query({ data: projectsPage([checkout]) }),
    );
    useProjectRequestCount.mockReturnValue(undefined);

    renderHome();

    // A missing measurement is not a zero: the card shows a placeholder and
    // says what it is waiting for.
    expect(screen.queryByText("0")).toBeNull();
    expect(
      screen.getByText("Requests in the last 30 days loading"),
    ).toBeInTheDocument();
  });

  it("renders real usage against the plan limit", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationPlan.mockReturnValue(query({ data: plan("pro", 3, 10) }));

    renderHome();

    // Usage rides on the plan chip's tooltip, keeping the reference's compact
    // organization row without dropping the real numbers.
    expect(screen.getByTitle("3 of 10 projects used")).toBeInTheDocument();
  });

  it("says unlimited instead of drawing a bar for an uncapped plan", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationPlan.mockReturnValue(query({ data: plan("free", 4, -1) }));

    renderHome();

    expect(screen.getByTitle("4 projects, unlimited")).toBeInTheDocument();
  });

  it("shows the card grid by default and switches to the table box", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationProjects.mockReturnValue(
      query({ data: projectsPage([checkout, inventory]) }),
    );

    renderHome();

    // The default presentation is the card grid, matching the Appwrite
    // reference, and its Requests row carries the project's real 30-day count.
    expect(
      screen.getByRole("heading", { name: "checkout-api" }),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Requests")).toHaveLength(2);
    expect(screen.getByText("12K")).toBeInTheDocument();
    expect(screen.getByText("0")).toBeInTheDocument();
    expect(screen.queryByText("N/A")).toBeNull();
    expect(screen.queryByRole("table")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "List view" }));

    // The box renders the same projects as the compact table: identity,
    // created date, and the visible-count footer.
    const table = screen.getByRole("table");
    expect(within(table).getByText("checkout-api")).toBeInTheDocument();
    expect(within(table).getByText("inventory-worker")).toBeInTheDocument();
    expect(within(table).getByText("Created 4 Apr 2026")).toBeInTheDocument();
    expect(
      within(table).getByText("Showing 2 of 2 projects"),
    ).toBeInTheDocument();
    // The metric row is card-only.
    expect(screen.queryByText("Requests")).toBeNull();
  });

  it("filters projects by name and offers a way back", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationProjects.mockReturnValue(
      query({ data: projectsPage([checkout, inventory]) }),
    );

    renderHome();
    // The visible-count footer lives in the table box, so switch to it first.
    fireEvent.click(screen.getByRole("button", { name: "List view" }));
    fireEvent.change(screen.getByRole("textbox", { name: "Search projects" }), {
      target: { value: "checkout" },
    });

    expect(screen.getByText("checkout-api")).toBeInTheDocument();
    expect(screen.queryByText("inventory-worker")).toBeNull();
    // The box footer reports the visible count against the organization total.
    expect(screen.getByText("Showing 1 of 2 projects")).toBeInTheDocument();

    fireEvent.change(screen.getByRole("textbox", { name: "Search projects" }), {
      target: { value: "nothing-matches" },
    });

    expect(
      screen.getByText("No projects match this search"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(screen.getByText("inventory-worker")).toBeInTheDocument();
  });

  it("opens the create project dialog from the toolbar", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    // A non-empty list, so the create controls are present: the organization
    // row quick action, the toolbar button, and the card's dashed action.
    useOrganizationProjects.mockReturnValue(
      query({ data: projectsPage([checkout]) }),
    );

    renderHome();
    // Three controls carry the name with a card on screen: the organization
    // row quick action first in the DOM, then the toolbar button, then the
    // card's dashed quick-create action.
    const createButtons = screen.getAllByRole("button", {
      name: "Create project",
    });
    expect(createButtons).toHaveLength(3);
    fireEvent.click(createButtons[createButtons.length - 1]);

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Create project" }),
    ).toBeInTheDocument();
  });

  it("opens the create project dialog from the organization row", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );

    renderHome();
    fireEvent.click(
      screen.getAllByRole("button", { name: "Create project" })[0],
    );

    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("offers the create action inside the empty state too", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );

    renderHome();

    expect(
      screen.getByText("No projects in this organization yet"),
    ).toBeInTheDocument();
    // The toolbar is withheld while the list is empty; the organization-row
    // quick action and the empty state keep the create path available.
    expect(
      screen.getAllByRole("button", { name: "Create project" }),
    ).toHaveLength(2);
  });

  it("marks the sections that do not exist yet instead of linking them", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );

    renderHome();

    expect(screen.getByText("Projects")).toHaveAttribute(
      "aria-current",
      "page",
    );
    for (const label of ["Marketplace", "Domains", "Settings"]) {
      const tab = screen.getByText(label).closest("[aria-disabled]");
      expect(tab).toHaveAttribute("aria-disabled", "true");
    }
    expect(
      screen.queryAllByRole("link", { name: /Marketplace|Domains/ }),
    ).toHaveLength(0);
  });

  it("always offers the organization switcher, even with a single organization", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    renderHome();
    const trigger = screen.getByRole("button", {
      name: "Switch organization",
    });
    expect(trigger).toBeInTheDocument();

    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Enter" });
    expect(screen.getByText("Current")).toBeInTheDocument();
  });

  it("switches organizations from the switcher", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme, beta]) }),
    );
    renderHome();
    const trigger = screen.getByRole("button", {
      name: "Switch organization",
    });
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Enter" });
    fireEvent.click(screen.getByText("Beta Labs"));
    expect(useOrganizationProjects).toHaveBeenLastCalledWith(beta.id);
  });

  it("explains an account with no organizations", () => {
    useOrganizations.mockReturnValue(query({ data: organizationsPage([]) }));

    renderHome();

    expect(
      screen.getByText("You are not in an organization yet"),
    ).toBeInTheDocument();
    // Nothing to create a project in, so the quick action is disabled.
    expect(
      screen.getByRole("button", { name: "Create project" }),
    ).toBeDisabled();
  });

  it("hides every create control from a member who may not create", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationProjects.mockReturnValue(
      query({ data: projectsPage([checkout], { canManage: false }) }),
    );

    renderHome();

    // The list is still the point of the page.
    expect(
      screen.getByRole("heading", { name: "checkout-api" }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create project" })).toBeNull();
  });

  it("says who can create the first project when the caller cannot", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationProjects.mockReturnValue(
      query({ data: projectsPage([], { canManage: false }) }),
    );

    renderHome();

    expect(
      screen.getByText(
        "An owner or admin of this organization can create the first project.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create project" })).toBeNull();
  });

  it("offers the rest of a long project list instead of dropping it", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    const fetchNextPage = vi.fn();
    useOrganizationProjects.mockReturnValue(
      query({
        data: projectsPage([checkout], { nextCursor: "cursor-2" }),
        hasNextPage: true,
        fetchNextPage,
      }),
    );

    renderHome();

    // The grid offers the same control as the table box.
    fireEvent.click(screen.getByRole("button", { name: "Load more projects" }));
    expect(fetchNextPage).toHaveBeenCalled();

    // The compact view says the count is partial, so it is not read as the
    // organization's total.
    fireEvent.click(screen.getByRole("button", { name: "List view" }));
    expect(screen.getByText("Showing 1 of 1 loaded")).toBeInTheDocument();
  });

  it("keeps Load more reachable when the search matches nothing yet", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    const fetchNextPage = vi.fn();
    useOrganizationProjects.mockReturnValue(
      query({
        data: projectsPage([checkout], { nextCursor: "cursor-2" }),
        hasNextPage: true,
        fetchNextPage,
      }),
    );

    renderHome();
    fireEvent.change(screen.getByLabelText("Search projects"), {
      target: { value: "not-on-this-page" },
    });

    // The match may live on a later page, so hiding the control here would make
    // that project unreachable.
    fireEvent.click(screen.getByRole("button", { name: "Load more projects" }));
    expect(fetchNextPage).toHaveBeenCalled();
  });

  it("offers the rest of a long organization list", () => {
    const fetchNextPage = vi.fn();
    useOrganizations.mockReturnValue(
      query({
        data: organizationsPage([acme], { nextCursor: "org-cursor-2" }),
        hasNextPage: true,
        fetchNextPage,
      }),
    );
    useOrganizationProjects.mockReturnValue(
      query({ data: projectsPage([checkout]) }),
    );

    renderHome();

    fireEvent.pointerDown(
      screen.getByRole("button", { name: "Switch organization" }),
      { button: 0, ctrlKey: false, pointerType: "mouse" },
    );
    fireEvent.click(
      screen.getByRole("menuitem", { name: "Load more organizations" }),
    );
    expect(fetchNextPage).toHaveBeenCalled();
  });

  it("shows skeletons while projects load", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationProjects.mockReturnValue(query({ isPending: true }));

    renderHome();

    expect(screen.queryByText("checkout-api")).toBeNull();
    // Permission is part of the list, so it is unknown until the list lands.
    // Offering a create control now would be a guess that can answer 403.
    expect(screen.queryByRole("button", { name: "Create project" })).toBeNull();
  });

  it("surfaces a failed project load with a working retry", () => {
    const refetch = vi.fn();
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useOrganizationProjects.mockReturnValue(
      query({ isError: true, error: new Error("boom"), refetch }),
    );

    renderHome();

    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(refetch).toHaveBeenCalled();
  });

  it("surfaces a failed organization load", () => {
    useOrganizations.mockReturnValue(
      query({ isError: true, error: new Error("boom"), refetch: vi.fn() }),
    );

    renderHome();

    expect(
      screen.getByText("Unable to load organizations"),
    ).toBeInTheDocument();
  });

  it("links to the admin console only for an instance admin role", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );
    useCurrentAccount.mockReturnValue(
      query({
        data: {
          account: {
            id: "acc-1",
            email: "owner@example.com",
            display_name: "Owner",
            email_verified: true,
            instance_role: "instance_owner",
            created_at: "2026-01-01T00:00:00Z",
          },
        },
      }),
    );

    renderHome();

    expect(
      screen.getByRole("link", { name: /Instance owner/ }),
    ).toHaveAttribute("href", "/admin");
  });

  it("hides the admin link for a plain member", () => {
    useOrganizations.mockReturnValue(
      query({ data: organizationsPage([acme]) }),
    );

    renderHome();

    expect(screen.queryByRole("link", { name: /Instance/ })).toBeNull();
  });
});
