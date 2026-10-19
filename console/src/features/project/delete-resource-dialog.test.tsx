import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DeleteResourceDialog } from "@/features/project/delete-resource-dialog";

// The dialog's contract is the gate: the destructive control stays disabled
// until the operator types the resource's name exactly, and a failed delete
// leaves the dialog open with the API's reason.
function renderDialog(
  props: Partial<Parameters<typeof DeleteResourceDialog>[0]> = {},
) {
  const onOpenChange = vi.fn();
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  render(
    <DeleteResourceDialog
      open
      onOpenChange={onOpenChange}
      resourceLabel="database"
      name="app"
      consequence="This deletes app and every table inside it."
      onConfirm={onConfirm}
      {...props}
    />,
  );
  return { onOpenChange, onConfirm };
}

const confirmButton = () =>
  screen.getByRole("button", { name: "Delete database" });

beforeEach(() => {
  vi.clearAllMocks();
});

describe("DeleteResourceDialog", () => {
  it("states what goes with the resource", () => {
    renderDialog();

    expect(
      screen.getByText("This deletes app and every table inside it."),
    ).toBeInTheDocument();
  });

  it("keeps the destructive control disabled until the name matches", () => {
    renderDialog();

    expect(confirmButton()).toBeDisabled();

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "ap" },
    });
    expect(confirmButton()).toBeDisabled();
    expect(screen.getByText("The name does not match yet.")).toBeInTheDocument();

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "app" },
    });
    expect(confirmButton()).toBeEnabled();
  });

  it("confirms, closes, and reports nothing on success", async () => {
    const { onConfirm, onOpenChange } = renderDialog();

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "app" },
    });
    fireEvent.click(confirmButton());

    await waitFor(() => expect(onConfirm).toHaveBeenCalledOnce());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("surfaces the API's reason and stays open", async () => {
    const onConfirm = vi
      .fn()
      .mockRejectedValue(new Error("database is still in use"));
    const { onOpenChange } = renderDialog({ onConfirm });

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "app" },
    });
    fireEvent.click(confirmButton());

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "database is still in use",
    );
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("forgets a typed name when it is dismissed", () => {
    renderDialog();

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "app" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    // The dialog stays mounted so Radix can animate it out; the field must not
    // carry the previous attempt into the next one.
    expect(screen.getByRole("textbox")).toHaveValue("");
    expect(confirmButton()).toBeDisabled();
  });
});
