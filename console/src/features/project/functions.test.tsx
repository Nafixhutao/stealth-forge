import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectFunctions } from "@/features/project/functions";

// The section is verified through its own contract: the figures in its header,
// the function each row describes, and what it shows for each read state. The
// query hooks are stubbed so the assertions are about this component, not the
// network.
const useProjectFunctions = vi.fn();
const useProjectUsage = vi.fn();

vi.mock("@/api/queries/projects", () => ({
  useProjectFunctions: (id: string | undefined) => useProjectFunctions(id),
  useProjectUsage: (id: string | undefined) => useProjectUsage(id),
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

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
    ...overrides,
  };
}

/** An infinite read's shape: the page plus the cursor that follows it. */
function listQuery(overrides: Partial<QueryStub> = {}): QueryStub {
  return query({
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: vi.fn(),
    ...overrides,
  });
}

function page(functions: unknown[], nextCursor?: string) {
  return {
    pages: [
      {
        functions,
        pagination: nextCursor ? { next_cursor: nextCursor } : {},
        can_manage: true,
      },
    ],
  };
}

const resize = {
  id: "11111111-1111-1111-1111-111111111111",
  name: "resize-image",
  runtime: "node-22",
  description: "Resizes uploads before they are stored",
  status: "active",
  artifact_used_bytes: 2_097_152,
  artifact_quota_bytes: 52_428_800,
  created_at: "2026-10-15T09:12:00.000Z",
};

const receipt = {
  id: "22222222-2222-2222-2222-222222222222",
  name: "send-receipt",
  runtime: "go-1.24",
  description: null,
  status: "disabled",
  artifact_used_bytes: 512,
  artifact_quota_bytes: 52_428_800,
  created_at: "2026-10-16T10:00:00.000Z",
};

const usage = {
  usage: {
    project_id: PROJECT_ID,
    captured_at: "2026-10-18T13:00:00.000Z",
    function_count: 4,
    function_artifact_bytes: 2_097_152,
    function_quota_bytes: 52_428_800,
    function_invocation_count_30d: 640,
    function_failure_count_30d: 3,
  },
};

function renderFunctions() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectFunctions projectId={PROJECT_ID} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useProjectFunctions.mockReturnValue(
    listQuery({ data: page([resize, receipt]) }),
  );
  useProjectUsage.mockReturnValue(query({ data: usage }));
});

describe("ProjectFunctions", () => {
  it("reports the function count and the invocation window, from the snapshot", () => {
    renderFunctions();

    // The invocations are labelled with their window: a bare total on a
    // project page reads as all-time.
    expect(
      screen.getByText("4 functions · 640 invocations in 30 days"),
    ).toBeInTheDocument();
  });

  it("keeps the header honest while the snapshot is still loading", () => {
    useProjectUsage.mockReturnValue(query({ isPending: true }));

    renderFunctions();

    expect(screen.getByText("The project's functions.")).toBeInTheDocument();
    expect(screen.queryByText(/invocations/)).toBeNull();
  });

  it("shows each function with its status, runtime, and artifact use", () => {
    renderFunctions();

    expect(screen.getByText("resize-image")).toBeInTheDocument();
    expect(
      screen.getByText("Resizes uploads before they are stored"),
    ).toBeInTheDocument();
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.getByText("node-22")).toBeInTheDocument();
    expect(screen.getByText("2.0 MB of 50 MB")).toBeInTheDocument();

    expect(screen.getByText("send-receipt")).toBeInTheDocument();
    expect(screen.getByText("Disabled")).toBeInTheDocument();
    expect(screen.getByText("go-1.24")).toBeInTheDocument();
    expect(screen.getByText("512 B of 50 MB")).toBeInTheDocument();
  });

  it("falls back to the created date when a function has no description", () => {
    renderFunctions();

    // The identity cell always carries a line of context, so a row without a
    // description is not a row with an empty second line.
    expect(
      screen.getByText("Created Oct 16, 2026 · 10:00"),
    ).toBeInTheDocument();
  });

  it("names the empty state and the next step", () => {
    useProjectFunctions.mockReturnValue(listQuery({ data: page([]) }));

    renderFunctions();

    expect(
      screen.getByText("No functions in this project yet"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "A function runs on demand. Create one from the API to see it listed here.",
      ),
    ).toBeInTheDocument();
  });

  it("shows the list's loading footprint rather than an empty list", () => {
    useProjectFunctions.mockReturnValue(listQuery({ isPending: true }));

    renderFunctions();

    expect(screen.getByText("Functions")).toBeInTheDocument();
    expect(screen.queryByText("resize-image")).toBeNull();
    expect(screen.queryByText("No functions in this project yet")).toBeNull();
  });

  it("prefers the description over the created date when one is set", () => {
    useProjectFunctions.mockReturnValue(listQuery({ data: page([resize]) }));

    renderFunctions();

    expect(
      screen.getByText("Resizes uploads before they are stored"),
    ).toBeInTheDocument();
    expect(screen.queryByText("Created Oct 15, 2026 · 09:12")).toBeNull();
  });

  it("surfaces a failed read with a working retry", () => {
    const refetch = vi.fn();
    useProjectFunctions.mockReturnValue(
      listQuery({ isError: true, error: new Error("boom"), refetch }),
    );

    renderFunctions();

    expect(screen.getByText("Unable to load functions")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(refetch).toHaveBeenCalled();
  });

  it("counts what it shows without offering a page that does not exist", () => {
    renderFunctions();

    expect(screen.getByText("Showing 2 functions")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("uses the singular when the list holds one function", () => {
    useProjectFunctions.mockReturnValue(listQuery({ data: page([resize]) }));

    renderFunctions();

    expect(screen.getByText("Showing 1 function")).toBeInTheDocument();
  });

  it("loads the next page from the cursor the API returned", () => {
    const fetchNextPage = vi.fn();
    useProjectFunctions.mockReturnValue(
      listQuery({
        data: page([resize, receipt], "cursor-2"),
        hasNextPage: true,
        fetchNextPage,
      }),
    );

    renderFunctions();

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(fetchNextPage).toHaveBeenCalled();
  });
});
