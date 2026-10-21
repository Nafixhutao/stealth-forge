import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CreateFunctionDialog } from "@/features/project/create-function-dialog";

// The dialog is verified by the metadata it sends: a slug name, the chosen
// runtime, and an entrypoint that follows the runtime until the operator edits
// it. It writes metadata only, so the copy says a deployment is still needed.
const createFunction = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/mutations/functions", () => ({
  useCreateFunction: () => createFunction,
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

function renderDialog() {
  const onOpenChange = vi.fn();
  render(
    <CreateFunctionDialog
      projectId={PROJECT_ID}
      open
      onOpenChange={onOpenChange}
    />,
  );
  return { onOpenChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  createFunction.mutateAsync.mockResolvedValue({
    function: { id: "f1", name: "send-receipt" },
  });
});

describe("CreateFunctionDialog", () => {
  it("says a deployment is still needed", () => {
    renderDialog();

    expect(
      screen.getByText(/it starts answering once a deployment is uploaded/i),
    ).toBeInTheDocument();
  });

  it("normalizes the name and defaults to Node with its entrypoint", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Function name"), {
      target: { value: "Send Receipt" },
    });

    expect(
      screen.getByText("Will be created as send-receipt"),
    ).toBeInTheDocument();
    expect(screen.getByLabelText("Runtime")).toHaveValue("node-22");
    expect(screen.getByLabelText("Entrypoint")).toHaveValue("src/main.js");
  });

  it("moves the entrypoint with the runtime until it is edited", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Runtime"), {
      target: { value: "python-3.13" },
    });
    expect(screen.getByLabelText("Entrypoint")).toHaveValue("main.py");

    // Once the operator names their own file, changing runtime must not
    // overwrite it.
    fireEvent.change(screen.getByLabelText("Entrypoint"), {
      target: { value: "app/handler.py" },
    });
    fireEvent.change(screen.getByLabelText("Runtime"), {
      target: { value: "go-1.24" },
    });
    expect(screen.getByLabelText("Entrypoint")).toHaveValue("app/handler.py");
  });

  it("sends the metadata the API requires, with the defaults stated", async () => {
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText("Function name"), {
      target: { value: "send-receipt" },
    });
    fireEvent.change(screen.getByLabelText("Runtime"), {
      target: { value: "python-3.13" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create function" }));

    await waitFor(() =>
      expect(createFunction.mutateAsync).toHaveBeenCalledWith({
        name: "send-receipt",
        runtime: "python-3.13",
        entrypoint: "main.py",
        commands: "",
        timeout_seconds: 15,
        enabled: true,
        logging: true,
      }),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("surfaces the API's reason and stays open", async () => {
    createFunction.mutateAsync.mockRejectedValue(
      new Error("name must use lowercase letters, numbers, and hyphens"),
    );
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText("Function name"), {
      target: { value: "send-receipt" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create function" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "name must use lowercase letters, numbers, and hyphens",
    );
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("restores the defaults when dismissed", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Function name"), {
      target: { value: "send-receipt" },
    });
    fireEvent.change(screen.getByLabelText("Runtime"), {
      target: { value: "go-1.24" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByLabelText("Function name")).toHaveValue("");
    expect(screen.getByLabelText("Runtime")).toHaveValue("node-22");
    expect(screen.getByLabelText("Entrypoint")).toHaveValue("src/main.js");
  });
});
