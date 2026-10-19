import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectSites } from "@/features/project/sites";

// The view is verified through its own contract: which figures the header
// reports, what each row shows, what it offers an owner, and what it does for
// each read state and for a second page. The query hooks are stubbed so the
// assertions are about this component, not about the network; the writes are
// stubbed for the same reason.
const useProjectSites = vi.fn();
const useProjectUsage = vi.fn();
const createSite = { mutateAsync: vi.fn(), isPending: false };
const deploySite = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/queries/projects", () => ({
  useProjectSites: (id: string | undefined) => useProjectSites(id),
  useProjectUsage: (id: string | undefined) => useProjectUsage(id),
}));

vi.mock("@/api/mutations/sites", () => ({
  useCreateSite: () => createSite,
  useDeploySite: () => deploySite,
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

type Site = {
  id: string;
  project_id: string;
  name: string;
  platform_hostname: string | null;
  framework: "static";
  enabled: boolean;
  status: "active" | "disabled";
  artifact_quota_bytes: number;
  artifact_used_bytes: number;
  artifact_reserved_bytes: number;
  active_deployment_id: string | null;
  created_at: string;
  updated_at: string;
};

type SitesQueryStub = {
  data?: unknown;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
};

function query(overrides: Partial<SitesQueryStub> = {}): SitesQueryStub {
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

function page(sites: Site[], canManage = true) {
  return { sites, can_manage: canManage };
}

function site(overrides: Partial<Site> = {}): Site {
  return {
    id: "44444444-4444-4444-4444-444444444441",
    project_id: PROJECT_ID,
    name: "docs",
    platform_hostname: "docs-stealth.example.com",
    framework: "static",
    enabled: true,
    status: "active",
    artifact_quota_bytes: 104_857_600,
    artifact_used_bytes: 1_048_576,
    artifact_reserved_bytes: 0,
    active_deployment_id: null,
    created_at: "2026-10-15T09:12:00.000Z",
    updated_at: "2026-10-15T09:12:00.000Z",
    ...overrides,
  };
}

const usage = {
  usage: {
    project_id: PROJECT_ID,
    captured_at: "2026-10-18T13:00:00.000Z",
    site_count: 2,
    site_artifact_bytes: 8_388_608,
    site_reserved_bytes: 1_048_576,
    site_quota_bytes: 104_857_600,
  },
};

function renderSites() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectSites projectId={PROJECT_ID} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useProjectSites.mockReturnValue(
    query({
      data: {
        pages: [
          page([
            site(),
            site({
              id: "44444444-4444-4444-4444-444444444442",
              name: "blog",
              platform_hostname: null,
              status: "disabled",
              enabled: false,
              artifact_used_bytes: 2_097_152,
              created_at: "2026-10-16T10:00:00.000Z",
            }),
          ]),
        ],
      },
    }),
  );
  useProjectUsage.mockReturnValue(query({ data: usage }));
});

describe("ProjectSites", () => {
  it("summarises the project's sites and artifact use from the snapshot", () => {
    renderSites();

    expect(screen.getByText("Sites")).toBeInTheDocument();
    expect(screen.getByText("2 sites · 8.0 MB of 100 MB")).toBeInTheDocument();
  });

  it("keeps a plain description until the snapshot lands", () => {
    useProjectUsage.mockReturnValue(query({ isPending: true }));

    renderSites();

    expect(screen.getByText("The project's sites.")).toBeInTheDocument();
  });

  it("shows each site's name, created time, status, and artifact use", () => {
    renderSites();

    expect(screen.getByText("docs")).toBeInTheDocument();
    expect(screen.getByText("blog")).toBeInTheDocument();
    expect(
      screen.getByText("Created Oct 15, 2026 · 09:12"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Created Oct 16, 2026 · 10:00"),
    ).toBeInTheDocument();
    expect(screen.getByText("1.0 MB of 100 MB")).toBeInTheDocument();
    expect(screen.getByText("2.0 MB of 100 MB")).toBeInTheDocument();
    expect(screen.getByText("Not deployed")).toBeInTheDocument();
    expect(screen.getByText("Disabled")).toBeInTheDocument();
  });

  it("calls an enabled site without a release not deployed", () => {
    useProjectSites.mockReturnValue(
      query({
        data: {
          pages: [page([site(), site({ active_deployment_id: "dep-1" })])],
        },
      }),
    );

    renderSites();

    // Only the row without a release says so; the other one is simply active.
    expect(screen.getAllByText("Not deployed")).toHaveLength(1);
    expect(screen.getAllByText("Active")).toHaveLength(1);
  });

  it("reports a disabled site as disabled even when a release exists", () => {
    useProjectSites.mockReturnValue(
      query({
        data: {
          pages: [
            page([
              site({
                status: "disabled",
                enabled: false,
                active_deployment_id: "dep-1",
              }),
            ]),
          ],
        },
      }),
    );

    renderSites();

    expect(screen.getByText("Disabled")).toBeInTheDocument();
    expect(screen.queryByText("Active")).toBeNull();
    expect(screen.queryByText("Not deployed")).toBeNull();
  });

  it("links a site's public address when the platform hostname is set", () => {
    renderSites();

    const link = screen.getByRole("link", {
      name: "docs-stealth.example.com",
    });
    expect(link).toHaveAttribute("href", "https://docs-stealth.example.com");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noreferrer");
  });

  it("says there is no address when the instance has no workload domain", () => {
    renderSites();

    // Only the site that has a hostname gets a link; the null one says so in
    // words instead of rendering an empty cell or a dead anchor.
    expect(screen.getByText("No address yet")).toBeInTheDocument();
    expect(screen.getAllByRole("link")).toHaveLength(1);
    expect(screen.queryByRole("link", { name: /blog/ })).toBeNull();
  });

  it("offers the empty state when the project has no sites", () => {
    useProjectSites.mockReturnValue(query({ data: { pages: [page([])] } }));

    renderSites();

    expect(
      screen.getByText("No sites in this project yet"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/serves built static files from one address/),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows the list skeleton while the read is in flight", () => {
    useProjectSites.mockReturnValue(query({ isPending: true }));

    const { container } = renderSites();

    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(
      0,
    );
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText("No sites in this project yet")).toBeNull();
  });

  it("surfaces a failed read with a working retry", () => {
    const refetch = vi.fn();
    useProjectSites.mockReturnValue(
      query({ isError: true, error: new Error("boom"), refetch }),
    );

    renderSites();

    expect(screen.getByText("Unable to load sites")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(refetch).toHaveBeenCalled();
  });

  it("closes the list with how many sites are on screen", () => {
    renderSites();

    expect(screen.getByText("Showing 2 sites")).toBeInTheDocument();
  });

  it("loads the next page when the API reports a cursor", () => {
    const fetchNextPage = vi.fn();
    useProjectSites.mockReturnValue(
      query({
        data: { pages: [page([site()])] },
        hasNextPage: true,
        fetchNextPage,
      }),
    );

    renderSites();

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(fetchNextPage).toHaveBeenCalled();
  });

  it("omits the load-more control when there is no next page", () => {
    renderSites();

    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("offers create and deploy to a caller who may manage sites", () => {
    renderSites();

    expect(
      screen.getByRole("button", { name: "Create site" }),
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: /^Deploy to / }),
    ).toHaveLength(2);
  });

  it("hides the write controls from a caller who may only read", () => {
    useProjectSites.mockReturnValue(
      query({ data: { pages: [page([site()], false)] } }),
    );

    renderSites();

    expect(screen.queryByRole("button", { name: "Create site" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: /^Deploy to / }),
    ).toBeNull();
    // The list itself is still the whole point of the section.
    expect(screen.getByText("docs")).toBeInTheDocument();
  });

  it("keeps the create action in the empty state for a manager", () => {
    useProjectSites.mockReturnValue(query({ data: { pages: [page([])] } }));

    renderSites();

    expect(
      screen.getByRole("button", { name: "Create site" }),
    ).toBeInTheDocument();
  });

  it("opens the create dialog from the header action", () => {
    renderSites();

    fireEvent.click(screen.getByRole("button", { name: "Create site" }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("Site name")).toBeInTheDocument();
  });

  it("opens the deploy dialog for the row it was clicked on", () => {
    renderSites();

    fireEvent.click(
      screen.getAllByRole("button", { name: /^Deploy to / })[1],
    );

    expect(
      screen.getByRole("heading", { name: "Deploy to blog" }),
    ).toBeInTheDocument();
  });
});
