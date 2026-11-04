import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DatabaseStudio } from "@/features/project/database-studio";

// The browser is verified through the contract it depends on: which table is
// selected, the schema it renders beside the grid, the typed cell values, and
// which write controls a viewer is offered. The query and mutation hooks are
// stubbed so the assertions are about this surface, not the network.
const useDatabase = vi.fn();
const useDatabaseTables = vi.fn();
const useDatabaseColumns = vi.fn();
const useDatabaseRows = vi.fn();
const createTable = { mutateAsync: vi.fn(), isPending: false };
const createRow = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/queries/databases", () => ({
  useDatabase: (...args: unknown[]) => useDatabase(...args),
  useDatabaseTables: (...args: unknown[]) => useDatabaseTables(...args),
  useDatabaseColumns: (...args: unknown[]) => useDatabaseColumns(...args),
  useDatabaseRows: (...args: unknown[]) => useDatabaseRows(...args),
}));

vi.mock("@/api/mutations/databases", () => ({
  useCreateDatabaseTable: () => createTable,
  useCreateDatabaseRow: () => createRow,
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";
const DATABASE_ID = "44444444-4444-4444-4444-444444444441";
const TABLE_ID = "55555555-5555-5555-5555-555555555551";

type InfiniteStub = {
  data?: unknown;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
};

function infinite(overrides: Partial<InfiniteStub> = {}): InfiniteStub {
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

const database = {
  id: DATABASE_ID,
  project_id: PROJECT_ID,
  name: "toko",
  created_at: "2026-10-15T09:20:00.000Z",
  updated_at: "2026-10-16T11:00:00.000Z",
};

function table(overrides: Record<string, unknown> = {}) {
  return {
    id: TABLE_ID,
    database_id: DATABASE_ID,
    project_id: PROJECT_ID,
    name: "orders",
    row_security: true,
    create_permissions: [],
    read_permissions: [],
    update_permissions: [],
    delete_permissions: [],
    created_at: "2026-10-15T09:20:00.000Z",
    updated_at: "2026-10-16T11:00:00.000Z",
    ...overrides,
  };
}

function column(
  key: string,
  type: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `${key}-id`,
    table_id: TABLE_ID,
    key,
    type,
    required: false,
    created_at: "2026-10-15T09:20:00.000Z",
    updated_at: "2026-10-16T11:00:00.000Z",
    ...extra,
  };
}

function row(data: Record<string, unknown>) {
  return {
    id: "66666666-6666-6666-6666-666666666661",
    table_id: TABLE_ID,
    project_id: PROJECT_ID,
    data,
    read_permissions: [],
    update_permissions: [],
    delete_permissions: [],
    created_at: "2026-10-15T09:20:00.000Z",
    updated_at: "2026-10-16T11:00:00.000Z",
  };
}

function tablesPage(canManage = true, items = [table()]) {
  return {
    pages: [
      {
        tables: items,
        pagination: { next_cursor: null },
        can_manage: canManage,
      },
    ],
  };
}

function renderStudio() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <DatabaseStudio projectId={PROJECT_ID} databaseId={DATABASE_ID} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useDatabase.mockReturnValue({
    data: { database },
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  });
  useDatabaseTables.mockReturnValue(infinite({ data: tablesPage(true) }));
  useDatabaseColumns.mockReturnValue(
    infinite({
      data: {
        pages: [
          {
            columns: [
              column("title", "text", { required: true }),
              column("amount", "integer"),
            ],
            pagination: { next_cursor: null },
          },
        ],
      },
    }),
  );
  useDatabaseRows.mockReturnValue(
    infinite({
      data: {
        pages: [
          {
            rows: [row({ title: "Kopi susu", amount: 45000 })],
            pagination: { next_cursor: null },
          },
        ],
      },
    }),
  );
});

describe("DatabaseStudio", () => {
  it("selects the first table and renders its schema and rows", () => {
    renderStudio();

    expect(screen.getByRole("heading", { name: "toko" })).toBeInTheDocument();
    expect(screen.getAllByText("orders").length).toBeGreaterThan(0);
    expect(screen.getAllByText("title").length).toBeGreaterThan(0);
    expect(screen.getAllByText("text").length).toBeGreaterThan(0);
    expect(screen.getAllByText("amount").length).toBeGreaterThan(0);
    expect(screen.getAllByText("integer").length).toBeGreaterThan(0);
    expect(screen.getByText("Kopi susu")).toBeInTheDocument();
    expect(screen.getByText("45000")).toBeInTheDocument();
  });

  it("offers create controls to a manager and hides them from a viewer", () => {
    const { unmount } = renderStudio();
    expect(
      screen.getByRole("button", { name: "Create table" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Add row" })).toBeInTheDocument();
    unmount();

    useDatabaseTables.mockReturnValue(infinite({ data: tablesPage(false) }));
    renderStudio();

    expect(
      screen.queryByRole("button", { name: "Create table" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Add row" }),
    ).not.toBeInTheDocument();
  });

  it("shows the empty table state and the selected-table prompt", () => {
    useDatabaseTables.mockReturnValue(infinite({ data: tablesPage(true, []) }));

    renderStudio();

    expect(screen.getByText("No tables yet")).toBeInTheDocument();
    expect(screen.getByText("Select a table")).toBeInTheDocument();
  });

  it("shows the empty row state", () => {
    useDatabaseRows.mockReturnValue(
      infinite({
        data: {
          pages: [{ rows: [], pagination: { next_cursor: null } }],
        },
      }),
    );

    renderStudio();

    expect(screen.getByText("No rows in this table yet")).toBeInTheDocument();
  });

  it("loads the next row page", () => {
    const fetchNextPage = vi.fn();
    useDatabaseRows.mockReturnValue(
      infinite({
        data: {
          pages: [
            {
              rows: [row({ title: "Kopi susu", amount: 45000 })],
              pagination: { next_cursor: "next" },
            },
          ],
        },
        hasNextPage: true,
        fetchNextPage,
      }),
    );

    renderStudio();

    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(fetchNextPage).toHaveBeenCalled();
  });
});
