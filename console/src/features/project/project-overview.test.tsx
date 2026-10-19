import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectOverview } from "@/features/project/project-overview";

// The view is verified through its own contract: which figures it shows, where
// its tiles link, and what it does for each read state. The query hooks are
// stubbed so the assertions are about this component, not about the network.
const useProject = vi.fn();
const useProjectUsage = vi.fn();
const useProjectUsageMetering = vi.fn();

vi.mock("@/api/queries/projects", () => ({
  useProject: (id: string | undefined) => useProject(id),
  useProjectUsage: (id: string | undefined) => useProjectUsage(id),
  useProjectUsageMetering: (id: string | undefined) =>
    useProjectUsageMetering(id),
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

type QueryStub = {
  data?: unknown;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
};

function query(overrides: Partial<QueryStub> = {}): QueryStub {
  return {
    data: undefined,
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...overrides,
  };
}

const usage = {
  usage: {
    project_id: PROJECT_ID,
    captured_at: "2026-10-18T13:00:00.000Z",
    application_users: 12,
    database_count: 3,
    database_table_count: 14,
    database_row_count: 4820,
    storage_file_count: 87,
    storage_bytes: 12_582_912,
    storage_quota_bytes: 536_870_912,
    function_count: 4,
    function_artifact_bytes: 2_097_152,
    function_quota_bytes: 52_428_800,
    site_count: 2,
    site_artifact_bytes: 8_388_608,
    site_reserved_bytes: 1_048_576,
    site_quota_bytes: 104_857_600,
    realtime_event_count: 1520,
    webhook_delivery_count_7d: 34,
    api_request_count_30d: 18_432,
    api_egress_bytes_30d: 41_943_040,
    function_invocation_count_30d: 640,
    function_failure_count_30d: 3,
    function_compute_ms_30d: 128_450,
  },
};

const metering = {
  project_id: PROJECT_ID,
  from: "2026-09-19",
  to: "2026-10-18",
  days: [
    {
      date: "2026-09-19",
      api_request_count: 300,
      api_egress_bytes: 1024,
      function_invocation_count: 2,
      function_failure_count: 0,
      function_compute_ms: 40,
    },
  ],
  totals: {
    date: "2026-10-18",
    api_request_count: 300,
    api_egress_bytes: 1024,
    function_invocation_count: 2,
    function_failure_count: 0,
    function_compute_ms: 40,
  },
};

function renderOverview() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectOverview projectId={PROJECT_ID} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useProject.mockReturnValue(
    query({
      data: {
        project: {
          id: PROJECT_ID,
          organization_id: "11111111-1111-1111-1111-111111111111",
          name: "nafi",
          created_at: "2026-10-15T09:12:00.000Z",
        },
      },
    }),
  );
  useProjectUsage.mockReturnValue(query({ data: usage }));
  useProjectUsageMetering.mockReturnValue(query({ data: metering }));
});

describe("ProjectOverview", () => {
  it("reports what the project holds, from the usage snapshot", () => {
    renderOverview();

    expect(screen.getByText("Databases")).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("14 tables · 4,820 rows")).toBeInTheDocument();
    expect(screen.getByText("12 MB of 512 MB")).toBeInTheDocument();
    expect(screen.getByText("640 runs in 30 days")).toBeInTheDocument();
    expect(screen.getByText("8.0 MB of 100 MB")).toBeInTheDocument();
    expect(screen.getByText("Accounts registered through this project")).toBeInTheDocument();
  });

  it("links each tile into the section it counts", () => {
    renderOverview();

    for (const section of ["databases", "storage", "functions", "sites"]) {
      expect(
        screen.getByRole("link", {
          name: new RegExp(section, "i"),
        }),
      ).toHaveAttribute("href", `/projects/${PROJECT_ID}/${section}`);
    }
  });

  it("keeps the rolling counters labelled as rolling", () => {
    renderOverview();

    // A bare "Requests" figure on a project page reads as all-time, so the
    // window is part of the label.
    expect(screen.getByText("18K API requests in 30 days")).toBeInTheDocument();
    expect(screen.getByText("40 MB egress")).toBeInTheDocument();
    expect(screen.getByText("Created Oct 15, 2026 · 09:12")).toBeInTheDocument();
  });

  it("shows the metering series from the window the API returned", () => {
    renderOverview();

    expect(screen.getByText("API requests")).toBeInTheDocument();
    expect(screen.getByText("300")).toBeInTheDocument();
    expect(screen.getByText("Sep 19, 2026 – Oct 18, 2026")).toBeInTheDocument();
  });

  it("does not claim zeroes while the snapshot is still loading", () => {
    useProjectUsage.mockReturnValue(query({ isPending: true }));

    renderOverview();

    // A zero is a fact; an unloaded figure is not, so the tile shows neither
    // the number nor its detail line until the read lands.
    expect(screen.getByText("Databases")).toBeInTheDocument();
    expect(screen.queryByText("14 tables · 4,820 rows")).toBeNull();
    expect(screen.queryByText("3")).toBeNull();
  });

  it("surfaces a failed usage read with a working retry", () => {
    const refetch = vi.fn();
    useProjectUsage.mockReturnValue(
      query({ isError: true, error: new Error("boom"), refetch }),
    );

    renderOverview();

    expect(screen.getByText("Unable to load project usage")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(refetch).toHaveBeenCalled();
  });

  it("keeps the snapshot on screen when only metering fails", () => {
    const refetch = vi.fn();
    useProjectUsageMetering.mockReturnValue(
      query({ isError: true, error: new Error("boom"), refetch }),
    );

    renderOverview();

    // One failed card does not take the rest of the page down with it.
    expect(screen.getByText("Unable to load metering")).toBeInTheDocument();
    expect(screen.getByText("14 tables · 4,820 rows")).toBeInTheDocument();
    expect(screen.queryByText("API requests")).toBeNull();
  });
});
