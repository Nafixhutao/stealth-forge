import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProjectDatabases } from "@/features/project/databases";

// The view is verified through its own contract: the figures it reports, the
// rows it draws, what it does for each read state, and which write controls it
// offers to whom. The query and mutation hooks are stubbed so the assertions
// are about this component, not about the network.
const useProjectDatabases = vi.fn();
const useProjectUsage = vi.fn();
const createDatabase = { mutateAsync: vi.fn(), isPending: false };
const deleteDatabase = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/queries/projects", () => ({
  useProjectDatabases: (id: string | undefined) => useProjectDatabases(id),
  useProjectUsage: (id: string | undefined) => useProjectUsage(id),
}));

vi.mock("@/api/mutations/databases", () => ({
  useCreateDatabase: () => createDatabase,
  useDeleteDatabase: () => deleteDatabase,
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

type Database = {
  id: string;
  project_id: string;
  name: string;
  created_at: string;
  updated_at: string;
};

type ListStub = {
  data?: unknown;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
};

function listQuery(overrides: Partial<ListStub> = {}): ListStub {
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

function page(databases: Database[], canManage = true, nextCursor?: string) {
  return {
    pages: [
      {
        databases,
        pagination: nextCursor ? { next_cursor: nextCursor } : {},
        can_manage: canManage,
      },
    ],
  };
}

function database(overrides: Partial<Database> = {}): Database {
  return {
    id: "d1111111-1111-1111-1111-111111111111",
    project_id: PROJECT_ID,
    name: "app",
    created_at: "2026-10-15T09:20:00.000Z",
    updated_at: "2026-10-16T11:00:00.000Z",
    ...overrides,
  };
}

const usage = {
  usage: {
    project_id: PROJECT_ID,
    captured_at: "2026-10-18T13:00:00.000Z",
    database_count: 2,
    database_table_count: 14,
    database_row_count: 4820,
  },
};

function renderDatabases() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectDatabases projectId={PROJECT_ID} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useProjectDatabases.mockReturnValue(
    listQuery({
      data: page([
        database(),
        database({
          id: "d2222222-2222-2222-2222-222222222222",
          name: "analytics",
          created_at: "2026-10-16T08:00:00.000Z",
        }),
      ]),
    }),
  );
  useProjectUsage.mockReturnValue(listQuery({ data: usage }));
});

describe("ProjectDatabases", () => {
  it("reports the project's databases, tables, and rows from the snapshot", () => {
    renderDatabases();

    expect(screen.getByText("Databases")).toBeInTheDocument();
    expect(screen.getByText("2 databases · 14 tables · 4,820 rows")).toBeInTheDocument();
  });

  it("keeps a plain description until the snapshot lands", () => {
    useProjectUsage.mockReturnValue(listQuery({ isPending: true }));

    renderDatabases();

    expect(
      screen.getByText("The project's databases and their tables."),
    ).toBeInTheDocument();
  });

  it("shows each database's name, created time, and id", () => {
    renderDatabases();

    expect(screen.getByText("app")).toBeInTheDocument();
    expect(screen.getByText("analytics")).toBeInTheDocument();
    expect(
      screen.getByText("Created Oct 15, 2026 · 09:20"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("d1111111-1111-1111-1111-111111111111"),
    ).toBeInTheDocument();
  });

  it("offers the empty state when the project has no databases", () => {
    useProjectDatabases.mockReturnValue(listQuery({ data: page([]) }));

    renderDatabases();

    expect(
      screen.getByText("No databases in this project yet"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows the list skeleton while the read is in flight", () => {
    useProjectDatabases.mockReturnValue(listQuery({ isPending: true }));

    const { container } = renderDatabases();

    expect(container.querySelectorAll(".animate-pulse").length).toBeGreaterThan(
      0,
    );
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("surfaces a failed read with a working retry", () => {
    const refetch = vi.fn();
    useProjectDatabases.mockReturnValue(
      listQuery({ isError: true, error: new Error("boom"), refetch }),
    );

    renderDatabases();

    expect(screen.getByText("Unable to load databases")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(refetch).toHaveBeenCalled();
  });

  it("closes the list with how many databases are on screen", () => {
    renderDatabases();

    expect(screen.getByText("Showing 2 databases")).toBeInTheDocument();
  });

  it("loads the next page when the API reports a cursor", () => {
    const fetchNextPage = vi.fn();
    useProjectDatabases.mockReturnValue(
      listQuery({
        data: page([database()], true, "cursor-2"),
        hasNextPage: true,
        fetchNextPage,
      }),
    );

    renderDatabases();

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(fetchNextPage).toHaveBeenCalled();
  });

  it("offers create and delete to a caller who may manage databases", () => {
    renderDatabases();

    expect(
      screen.getByRole("button", { name: "Create database" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Delete database app" }),
    ).toBeInTheDocument();
  });

  it("hides the write controls from a caller who may only read", () => {
    useProjectDatabases.mockReturnValue(
      listQuery({ data: page([database()], false) }),
    );

    renderDatabases();

    expect(
      screen.queryByRole("button", { name: "Create database" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Delete database app" }),
    ).toBeNull();
    // The list itself is still the whole point of the section.
    expect(screen.getByText("app")).toBeInTheDocument();
  });

  it("keeps the create action in the empty state for a manager", () => {
    useProjectDatabases.mockReturnValue(listQuery({ data: page([]) }));

    renderDatabases();

    expect(
      screen.getByRole("button", { name: "Create database" }),
    ).toBeInTheDocument();
  });

  it("opens the create dialog from the header action", () => {
    renderDatabases();

    fireEvent.click(screen.getByRole("button", { name: "Create database" }));

    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("Database name")).toBeInTheDocument();
  });

  it("opens the delete confirmation for the row it was clicked on", () => {
    renderDatabases();

    fireEvent.click(screen.getByRole("button", { name: "Delete database analytics" }));

    expect(
      screen.getByRole("heading", { name: "Delete database" }),
    ).toBeInTheDocument();
    // The consequence names the row, so the operator confirms the right one.
    expect(
      screen.getByText(/This deletes analytics and every table, index, and row/),
    ).toBeInTheDocument();
  });
});
