import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { StorageBucket } from "@/api/types";
import { ProjectStorage } from "@/features/project/storage";

// The view is verified through its own contract: the figures it shows, the rows
// it draws, and what it does for each read state. The query hooks are stubbed
// so the assertions are about this component, not about the network.
const useProjectBuckets = vi.fn();
const useProjectUsage = vi.fn();
const createBucket = { mutateAsync: vi.fn(), isPending: false };
const deleteBucket = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/queries/projects", () => ({
  useProjectBuckets: (id: string | undefined) => useProjectBuckets(id),
  useProjectUsage: (id: string | undefined) => useProjectUsage(id),
}));

vi.mock("@/api/mutations/storage", () => ({
  useCreateBucket: () => createBucket,
  useDeleteBucket: () => deleteBucket,
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

type UsageStub = {
  data?: unknown;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
};

type BucketsStub = {
  data?: {
    pages: Array<{ buckets: StorageBucket[]; can_manage?: boolean }>;
  };
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
};

function usageQuery(overrides: Partial<UsageStub> = {}): UsageStub {
  return {
    data: undefined,
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...overrides,
  };
}

function bucketsQuery(overrides: Partial<BucketsStub> = {}): BucketsStub {
  return {
    data: { pages: [{ buckets: [] }] },
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

function bucket(overrides: Partial<StorageBucket> = {}): StorageBucket {
  return {
    id: "44444444-4444-4444-4444-444444444441",
    project_id: PROJECT_ID,
    name: "uploads",
    file_security: true,
    create_permissions: [],
    read_permissions: [],
    update_permissions: [],
    delete_permissions: [],
    max_file_size_bytes: 10_485_760,
    quota_bytes: 536_870_912,
    used_bytes: 12_582_912,
    created_at: "2026-10-15T09:12:00.000Z",
    updated_at: "2026-10-16T09:12:00.000Z",
    ...overrides,
  };
}

const usage = {
  usage: {
    storage_file_count: 87,
    storage_bytes: 12_582_912,
    storage_quota_bytes: 536_870_912,
  },
};

function renderStorage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectStorage projectId={PROJECT_ID} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useProjectBuckets.mockReturnValue(bucketsQuery());
  useProjectUsage.mockReturnValue(usageQuery({ data: usage }));
});

describe("ProjectStorage", () => {
  it("summarises the project's storage from the usage snapshot", () => {
    renderStorage();

    expect(screen.getByText("Storage")).toBeInTheDocument();
    expect(screen.getByText("87 files · 12 MB of 512 MB")).toBeInTheDocument();
  });

  it("keeps the heading plain until the snapshot lands", () => {
    useProjectUsage.mockReturnValue(usageQuery({ isPending: true }));

    renderStorage();

    expect(
      screen.getByText("The project's storage buckets."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/MB of/)).toBeNull();
  });

  it("lists each bucket with its created date and quota use", () => {
    useProjectBuckets.mockReturnValue(
      bucketsQuery({ data: { pages: [{ buckets: [bucket()] }] } }),
    );

    renderStorage();

    expect(screen.getByText("uploads")).toBeInTheDocument();
    expect(
      screen.getByText("Created Oct 15, 2026 · 09:12"),
    ).toBeInTheDocument();
    expect(screen.getByText("12 MB of 512 MB")).toBeInTheDocument();
  });

  it("shows the raw bucket id for support and copy", () => {
    const row = bucket({ id: "55555555-5555-5555-5555-555555555551" });
    useProjectBuckets.mockReturnValue(
      bucketsQuery({ data: { pages: [{ buckets: [row] }] } }),
    );

    renderStorage();

    expect(screen.getByText(row.id)).toBeInTheDocument();
  });

  it("renders the rows the API returned across every page", () => {
    useProjectBuckets.mockReturnValue(
      bucketsQuery({
        data: {
          pages: [
            { buckets: [bucket({ id: "a", name: "uploads" })] },
            { buckets: [bucket({ id: "b", name: "exports" })] },
          ],
        },
      }),
    );

    renderStorage();

    expect(screen.getByText("uploads")).toBeInTheDocument();
    expect(screen.getByText("exports")).toBeInTheDocument();
    expect(screen.getByText("Showing 2 buckets")).toBeInTheDocument();
  });

  it("says so when the project has no buckets", () => {
    renderStorage();

    expect(
      screen.getByText("No buckets in this project yet"),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "A bucket holds the project's files. Create one, then upload files to it.",
      ),
    ).toBeInTheDocument();
  });

  it("holds the list's footprint while buckets load", () => {
    useProjectBuckets.mockReturnValue(bucketsQuery({ isPending: true }));

    renderStorage();

    // A skeleton stands in for the box, so the section does not claim to be
    // empty before the read lands.
    expect(screen.queryByRole("table")).toBeNull();
    expect(screen.queryByText("uploads")).toBeNull();
    expect(screen.queryByText("No buckets in this project yet")).toBeNull();
  });

  it("offers create and delete to a caller who may manage buckets", () => {
    useProjectBuckets.mockReturnValue(
      bucketsQuery({
        data: {
          pages: [{ buckets: [bucket()], can_manage: true }],
        },
      }),
    );

    renderStorage();

    expect(
      screen.getByRole("button", { name: "Create bucket" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Delete bucket uploads" }),
    ).toBeInTheDocument();
  });

  it("hides the write controls from a caller who may only read", () => {
    useProjectBuckets.mockReturnValue(
      bucketsQuery({
        data: {
          pages: [{ buckets: [bucket()], can_manage: false }],
        },
      }),
    );

    renderStorage();

    expect(
      screen.queryByRole("button", { name: "Create bucket" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Delete bucket uploads" }),
    ).toBeNull();
    expect(screen.getByText("uploads")).toBeInTheDocument();
  });

  it("opens the create dialog from the header action", () => {
    useProjectBuckets.mockReturnValue(
      bucketsQuery({
        data: {
          pages: [{ buckets: [bucket()], can_manage: true }],
        },
      }),
    );

    renderStorage();

    fireEvent.click(screen.getByRole("button", { name: "Create bucket" }));

    expect(screen.getByLabelText("Bucket name")).toBeInTheDocument();
  });

  it("asks for the bucket's name before deleting it", () => {
    useProjectBuckets.mockReturnValue(
      bucketsQuery({
        data: {
          pages: [{ buckets: [bucket()], can_manage: true }],
        },
      }),
    );

    renderStorage();

    fireEvent.click(screen.getByRole("button", { name: "Delete bucket uploads" }));

    expect(
      screen.getByRole("heading", { name: "Delete bucket" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/This deletes uploads and every file stored in it/),
    ).toBeInTheDocument();
  });

  it("surfaces a failed read with a working retry", () => {
    const refetch = vi.fn();
    useProjectBuckets.mockReturnValue(
      bucketsQuery({ isError: true, error: new Error("boom"), refetch }),
    );

    renderStorage();

    expect(screen.getByText("Unable to load buckets")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(refetch).toHaveBeenCalled();
  });

  it("counts the buckets on screen", () => {
    useProjectBuckets.mockReturnValue(
      bucketsQuery({ data: { pages: [{ buckets: [bucket()] }] } }),
    );

    renderStorage();

    expect(screen.getByText("Showing 1 bucket")).toBeInTheDocument();
  });

  it("offers the next page only when the API reports one, and loads it", () => {
    const fetchNextPage = vi.fn();
    useProjectBuckets.mockReturnValue(
      bucketsQuery({
        data: { pages: [{ buckets: [bucket()] }] },
        hasNextPage: true,
        fetchNextPage,
      }),
    );

    renderStorage();

    const loadMore = screen.getByRole("button", { name: "Load more" });
    fireEvent.click(loadMore);
    expect(fetchNextPage).toHaveBeenCalledTimes(1);
  });

  it("does not offer a next page when the API reports none", () => {
    renderStorage();

    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });
});
