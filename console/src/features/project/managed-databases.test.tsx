import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import { ProjectManagedDatabases } from "@/features/project/managed-databases";

// The section is verified through its contract: the connection values it shows,
// the on-demand password reveal, and the states a project without a database
// (or an installation without the feature) sees.
const useManagedDatabases = vi.fn();
const useManagedDatabaseBackups = vi.fn();
const createDatabase = { mutateAsync: vi.fn(), isPending: false };
const deleteDatabase = { mutateAsync: vi.fn(), isPending: false };
const revealPassword = { mutateAsync: vi.fn(), isPending: false };
const rotatePassword = { mutateAsync: vi.fn(), isPending: false };
const restoreDatabase = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/queries/managed-databases", () => ({
  useManagedDatabases: (id: string | undefined) => useManagedDatabases(id),
}));

vi.mock("@/api/queries/managed-database-backups", () => ({
  useManagedDatabaseBackups: () => useManagedDatabaseBackups(),
  useRestoreManagedDatabase: () => restoreDatabase,
}));

vi.mock("@/api/mutations/managed-databases", () => ({
  useCreateManagedDatabase: () => createDatabase,
  useDeleteManagedDatabase: () => deleteDatabase,
  useRevealManagedDatabasePassword: () => revealPassword,
  useRotateManagedDatabasePassword: () => rotatePassword,
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

const database = {
  id: "77777777-7777-7777-7777-777777777771",
  project_id: PROJECT_ID,
  name: "toko",
  status: "ready",
  host: "tenant-mysql",
  port: 3306,
  database_name: "mdb_0123456789abcdef01234567",
  username: "u_0123456789abcdef01234567",
  phpmyadmin_url: "https://db.example.test/",
  size_bytes: 12 * 1024 * 1024,
  connection_count: 2,
  connection_limit: 10,
  storage_limit_bytes: 256 * 1024 * 1024,
  write_locked: false,
  created_at: "2026-11-02T08:00:00.000Z",
  updated_at: "2026-11-02T08:00:00.000Z",
};

function query(overrides: Record<string, unknown> = {}) {
  return {
    data: { databases: [database], can_manage: true },
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...overrides,
  };
}

function renderSection() {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectManagedDatabases projectId={PROJECT_ID} />
    </QueryClientProvider>,
  );
}

const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  useManagedDatabases.mockReturnValue(query());
  useManagedDatabaseBackups.mockReturnValue({
    data: { backups: [], latest_restore: undefined },
    isPending: false,
    isError: false,
    error: null,
  });
  restoreDatabase.mutateAsync.mockResolvedValue({
    restore: { status: "queued" },
  });
  revealPassword.mutateAsync.mockResolvedValue({ password: "rahasia-sekali" });
  rotatePassword.mutateAsync.mockResolvedValue({ password: "password-baru" });
});

describe("ProjectManagedDatabases", () => {
  it("shows the real MySQL connection values and the phpMyAdmin door", () => {
    renderSection();

    expect(screen.getByText("tenant-mysql")).toBeInTheDocument();
    expect(screen.getByText("3306")).toBeInTheDocument();
    expect(screen.getAllByText(database.database_name).length).toBeGreaterThan(
      0,
    );
    expect(screen.getAllByText(database.username).length).toBeGreaterThan(0);
    expect(
      screen.getByRole("link", { name: /open phpmyadmin/i }),
    ).toHaveAttribute("href", "https://db.example.test/");
    expect(screen.getByText("ready")).toBeInTheDocument();
    expect(screen.getByText(/of 256 MB/)).toBeInTheDocument();
    expect(screen.getByText("2 of 10")).toBeInTheDocument();
  });

  it("reveals the password only after the audited action", async () => {
    renderSection();

    expect(screen.queryByText("rahasia-sekali")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Reveal password" }));

    await waitFor(() =>
      expect(screen.getByText("rahasia-sekali")).toBeInTheDocument(),
    );
    expect(revealPassword.mutateAsync).toHaveBeenCalledWith(database.id);
  });

  it("copies an environment block only after the password is revealed", async () => {
    renderSection();

    fireEvent.click(screen.getByRole("button", { name: "Reveal password" }));
    await waitFor(() =>
      expect(screen.getByText("rahasia-sekali")).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Copy .env block" }));

    await waitFor(() => expect(writeText).toHaveBeenCalled());
    const block = String(writeText.mock.calls[0][0]);
    expect(block).toContain("DB_HOST=tenant-mysql");
    expect(block).toContain("DB_NAME=" + database.database_name);
    expect(block).toContain("DB_PASSWORD=rahasia-sekali");
  });

  it("rotates the password and shows the new value", async () => {
    renderSection();

    fireEvent.click(screen.getByRole("button", { name: "Rotate password" }));
    const confirm = screen
      .getAllByRole("button", { name: "Rotate password" })
      .at(-1);
    if (!confirm) throw new Error("rotate confirmation was not rendered");
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(rotatePassword.mutateAsync).toHaveBeenCalledWith(database.id),
    );
    await waitFor(() =>
      expect(screen.getByText("password-baru")).toBeInTheDocument(),
    );
  });

  it("invites a project without a database to create the free one", () => {
    useManagedDatabases.mockReturnValue(
      query({ data: { databases: [], can_manage: true } }),
    );

    renderSection();

    expect(screen.getByText("No MySQL database yet")).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: "Create database" }).length,
    ).toBeGreaterThan(0);
  });

  it("explains an installation where the feature is disabled", () => {
    useManagedDatabases.mockReturnValue(
      query({
        data: undefined,
        isError: true,
        error: new ApiError(
          "managed databases are not enabled",
          503,
          "feature_disabled",
        ),
      }),
    );

    renderSection();

    expect(
      screen.getByText("MySQL hosting is not enabled"),
    ).toBeInTheDocument();
  });
});
