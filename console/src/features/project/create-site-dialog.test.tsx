import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CreateSiteDialog } from "@/features/project/create-site-dialog";

// The dialog is verified by what it sends and what it hands back: the
// normalized name, the defaults it states explicitly, and the created site the
// caller needs in order to offer a deployment next.
const createSite = { mutateAsync: vi.fn(), isPending: false };

vi.mock("@/api/mutations/sites", () => ({
  useCreateSite: () => createSite,
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

const created = {
  site: {
    id: "44444444-4444-4444-4444-444444444441",
    project_id: PROJECT_ID,
    name: "docs",
    platform_hostname: "docs.example.com",
    framework: "static" as const,
    enabled: true,
    status: "active" as const,
    artifact_quota_bytes: 1_073_741_824,
    artifact_used_bytes: 0,
    artifact_reserved_bytes: 0,
    active_deployment_id: null,
    created_at: "2026-10-18T13:00:00.000Z",
    updated_at: "2026-10-18T13:00:00.000Z",
  },
};

function renderDialog(
  props: Partial<Parameters<typeof CreateSiteDialog>[0]> = {},
) {
  const onOpenChange = vi.fn();
  const onCreated = vi.fn();
  render(
    <CreateSiteDialog
      projectId={PROJECT_ID}
      open
      onOpenChange={onOpenChange}
      onCreated={onCreated}
      {...props}
    />,
  );
  return { onOpenChange, onCreated };
}

beforeEach(() => {
  vi.clearAllMocks();
  createSite.mutateAsync.mockResolvedValue(created);
});

describe("CreateSiteDialog", () => {
  it("normalizes the name and says what will be created", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Site name"), {
      target: { value: "Docs Site" },
    });

    expect(
      screen.getByText("Will be created as docs-site"),
    ).toBeInTheDocument();
  });

  it("states the rule instead of failing on submit", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Site name"), {
      target: { value: "d" },
    });

    expect(
      screen.getByText(
        "Use 2 to 63 characters: lower-case letters, numbers, and hyphens.",
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create site" })).toBeDisabled();
  });

  it("creates the site with the name it showed, then hands it back", async () => {
    const { onOpenChange, onCreated } = renderDialog();

    fireEvent.change(screen.getByLabelText("Site name"), {
      target: { value: "Docs" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create site" }));

    await waitFor(() =>
      expect(createSite.mutateAsync).toHaveBeenCalledWith({
        name: "docs",
        framework: "static",
        enabled: true,
      }),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(onCreated).toHaveBeenCalledWith(created.site);
  });

  it("surfaces the API's reason and stays open", async () => {
    createSite.mutateAsync.mockRejectedValue(
      new Error("the organization's site limit has been reached"),
    );
    const { onOpenChange, onCreated } = renderDialog();

    fireEvent.change(screen.getByLabelText("Site name"), {
      target: { value: "docs" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create site" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "the organization's site limit has been reached",
    );
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(onCreated).not.toHaveBeenCalled();
  });

  it("clears a cancelled attempt so it cannot leak into the next one", () => {
    renderDialog();

    fireEvent.change(screen.getByLabelText("Site name"), {
      target: { value: "docs" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByLabelText("Site name")).toHaveValue("");
  });
});
