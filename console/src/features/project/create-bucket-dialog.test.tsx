import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CreateBucketDialog } from "@/features/project/create-bucket-dialog";

// The bucket name becomes part of the bucket's identity, so the dialog slugifies
// it; per-file permissions are exposed because they decide how every file in the
// bucket is authorised, and the API defaults them on.
const createBucket = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/mutations/storage", () => ({
  useCreateBucket: () => createBucket,
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

function renderDialog() {
  const onOpenChange = vi.fn();
  render(
    <CreateBucketDialog
      projectId={PROJECT_ID}
      open
      onOpenChange={onOpenChange}
    />,
  );
  return { onOpenChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  createBucket.mutateAsync.mockResolvedValue({
    bucket: { id: "b1", name: "avatars" },
  });
});

describe("CreateBucketDialog", () => {
  it("normalizes the name to a DNS label and says so", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Bucket name"), {
      target: { value: "User Avatars" },
    });

    expect(
      screen.getByText("Will be created as user-avatars"),
    ).toBeInTheDocument();
  });

  it("sends per-file permissions on by default", async () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Bucket name"), {
      target: { value: "avatars" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create bucket" }));

    await waitFor(() =>
      expect(createBucket.mutateAsync).toHaveBeenCalledWith({
        name: "avatars",
        file_security: true,
      }),
    );
  });

  it("sends the operator's choice when per-file permissions are turned off", async () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Bucket name"), {
      target: { value: "avatars" },
    });
    fireEvent.click(screen.getByRole("checkbox"));
    expect(
      screen.getByText(
        "Every file in the bucket shares the bucket's permissions.",
      ),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create bucket" }));

    await waitFor(() =>
      expect(createBucket.mutateAsync).toHaveBeenCalledWith({
        name: "avatars",
        file_security: false,
      }),
    );
  });

  it("surfaces the API's reason and stays open", async () => {
    createBucket.mutateAsync.mockRejectedValue(
      new Error("a bucket with this name already exists"),
    );
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText("Bucket name"), {
      target: { value: "avatars" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create bucket" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "a bucket with this name already exists",
    );
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("restores the defaults when dismissed", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Bucket name"), {
      target: { value: "avatars" },
    });
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByLabelText("Bucket name")).toHaveValue("");
    expect(screen.getByRole("checkbox")).toBeChecked();
  });
});
