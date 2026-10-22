import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CreateDatabaseDialog } from "@/features/project/create-database-dialog";

// The dialog is verified by what it sends and what it refuses to send: a
// database name is free text between 2 and 120 characters, so it is trimmed but
// not slugged, and the API's reason is surfaced verbatim.
const createDatabase = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/mutations/databases", () => ({
  useCreateDatabase: () => createDatabase,
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

function renderDialog() {
  const onOpenChange = vi.fn();
  render(
    <CreateDatabaseDialog
      projectId={PROJECT_ID}
      open
      onOpenChange={onOpenChange}
    />,
  );
  return { onOpenChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  createDatabase.mutateAsync.mockResolvedValue({
    database: { id: "d1", name: "app" },
  });
});

describe("CreateDatabaseDialog", () => {
  it("keeps the name as typed instead of forcing a slug", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Database name"), {
      target: { value: "Customer Orders" },
    });

    expect(screen.getByLabelText("Database name")).toHaveValue(
      "Customer Orders",
    );
    expect(
      screen.getByRole("button", { name: "Create database" }),
    ).toBeEnabled();
  });

  it("refuses a name shorter than two characters", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Database name"), {
      target: { value: "a" },
    });

    expect(screen.getByText("Use 2 to 120 characters.")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create database" }),
    ).toBeDisabled();
  });

  it("trims the name it sends", async () => {
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText("Database name"), {
      target: { value: "  app  " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create database" }));

    await waitFor(() =>
      expect(createDatabase.mutateAsync).toHaveBeenCalledWith({ name: "app" }),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("surfaces the API's reason and stays open", async () => {
    createDatabase.mutateAsync.mockRejectedValue(
      new Error("a database with this name already exists"),
    );
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText("Database name"), {
      target: { value: "app" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create database" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "a database with this name already exists",
    );
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("clears a cancelled attempt", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Database name"), {
      target: { value: "app" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByLabelText("Database name")).toHaveValue("");
  });
});
