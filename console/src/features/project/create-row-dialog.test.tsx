import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { DatabaseColumnType, type components } from "@/api/generated/schema";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CreateRowDialog } from "@/features/project/create-row-dialog";

// The dialog is verified by the shape it sends: each field is converted to the
// declared column type so the API's schema validation accepts it, and an
// untouched optional field is omitted so the server applies the column default.
const createRow = { mutateAsync: vi.fn(), isPending: false };
const toastSuccess = vi.hoisted(() => vi.fn());

vi.mock("@/api/mutations/databases", () => ({
  useCreateDatabaseRow: () => createRow,
}));

vi.mock("sonner", () => ({ toast: { success: toastSuccess, error: vi.fn() } }));

const COLUMNS: components["schemas"]["DatabaseColumn"][] = [
  {
    id: "c1",
    table_id: "t1",
    key: "title",
    type: DatabaseColumnType.text,
    required: true,
    created_at: "2026-10-15T09:20:00.000Z",
    updated_at: "2026-10-16T11:00:00.000Z",
  },
  {
    id: "c2",
    table_id: "t1",
    key: "amount",
    type: DatabaseColumnType.integer,
    required: true,
    created_at: "2026-10-15T09:20:00.000Z",
    updated_at: "2026-10-16T11:00:00.000Z",
  },
  {
    id: "c3",
    table_id: "t1",
    key: "status",
    type: DatabaseColumnType.text,
    required: false,
    default: "pending",
    created_at: "2026-10-15T09:20:00.000Z",
    updated_at: "2026-10-16T11:00:00.000Z",
  },
];

function renderDialog() {
  const onOpenChange = vi.fn();
  render(
    <CreateRowDialog
      projectId="p1"
      databaseId="d1"
      tableId="t1"
      tableName="orders"
      columns={COLUMNS}
      open
      onOpenChange={onOpenChange}
    />,
  );
  return { onOpenChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  createRow.mutateAsync.mockResolvedValue({ row: { id: "r1" } });
});

describe("CreateRowDialog", () => {
  it("converts declared types and omits untouched optional fields", async () => {
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText(/^title/), {
      target: { value: "Kopi susu" },
    });
    fireEvent.change(screen.getByLabelText(/^amount/), {
      target: { value: "45000" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add row" }));

    await waitFor(() =>
      expect(createRow.mutateAsync).toHaveBeenCalledWith({
        data: { title: "Kopi susu", amount: 45000 },
      }),
    );
    await waitFor(() =>
      expect(toastSuccess).toHaveBeenCalledWith("Added a row to orders"),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("rejects a fractional integer before calling the API", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText(/^amount/), {
      target: { value: "4.5" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add row" }));

    expect(
      screen.getByText("amount must be a whole number."),
    ).toBeInTheDocument();
    expect(createRow.mutateAsync).not.toHaveBeenCalled();
  });

  it("surfaces the API's rejection and stays open", async () => {
    createRow.mutateAsync.mockRejectedValue(
      new Error('title: missing required value: "title"'),
    );
    renderDialog();

    fireEvent.change(screen.getByLabelText(/^amount/), {
      target: { value: "45000" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Add row" }));

    await waitFor(() =>
      expect(
        screen.getByText('title: missing required value: "title"'),
      ).toBeInTheDocument(),
    );
  });
});
