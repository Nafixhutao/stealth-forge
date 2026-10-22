import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { DeploySiteDialog } from "@/features/project/deploy-site-dialog";
import { SiteFramework, SiteStatus } from "@/api/generated/schema";

// The dialog is verified by what it sends: the chosen file, for the site whose
// row it was opened from. Extension validation is client-side because the API
// rejects those four shapes; the size hint is not a block, because only the
// instance knows its own ceiling.
const deploySite = { mutateAsync: vi.fn(), isPending: false };
// `vi.mock` is hoisted above the imports, so the spy has to be hoisted with it.
const { toastSuccess } = vi.hoisted(() => ({ toastSuccess: vi.fn() }));

vi.mock("@/api/mutations/sites", () => ({
  useDeploySite: () => deploySite,
}));

vi.mock("sonner", () => ({ toast: { success: toastSuccess } }));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

const site = {
  id: "44444444-4444-4444-4444-444444444441",
  project_id: PROJECT_ID,
  name: "docs",
  platform_hostname: "docs.example.com",
  framework: SiteFramework.static,
  enabled: true,
  status: SiteStatus.active,
  artifact_quota_bytes: 1_073_741_824,
  artifact_used_bytes: 0,
  artifact_reserved_bytes: 0,
  active_deployment_id: null,
  created_at: "2026-10-18T13:00:00.000Z",
  updated_at: "2026-10-18T13:00:00.000Z",
};

function renderDialog() {
  const onOpenChange = vi.fn();
  render(
    <DeploySiteDialog
      projectId={PROJECT_ID}
      site={site}
      open
      onOpenChange={onOpenChange}
    />,
  );
  return { onOpenChange };
}

function chooseFile(name: string, bytes = "archive") {
  const file = new File([bytes], name, { type: "application/zip" });
  fireEvent.change(screen.getByLabelText("Site archive"), {
    target: { files: [file] },
  });
  return file;
}

beforeEach(() => {
  vi.clearAllMocks();
  deploySite.mutateAsync.mockResolvedValue({ deployment: { id: "dep-1" } });
});

describe("DeploySiteDialog", () => {
  it("names the site it publishes to and where it will answer", () => {
    renderDialog();

    expect(
      screen.getByRole("heading", { name: "Deploy to docs" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/docs\.example\.com/)).toBeInTheDocument();
  });

  it("rejects an archive the API does not accept", () => {
    renderDialog();

    chooseFile("site.rar");

    expect(
      screen.getByText("Use a .zip, .tar, .tar.gz, or .tgz archive."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deploy" })).toBeDisabled();
  });

  it("shows the chosen archive's name and size", () => {
    renderDialog();

    chooseFile("site.zip", "x".repeat(2048));

    expect(screen.getByText("site.zip · 2.0 KB")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Deploy" })).toBeEnabled();
  });

  it("uploads the archive to the site it was opened for", async () => {
    const { onOpenChange } = renderDialog();
    const file = chooseFile("site.tar.gz");

    fireEvent.click(screen.getByRole("button", { name: "Deploy" }));

    await waitFor(() =>
      expect(deploySite.mutateAsync).toHaveBeenCalledWith({
        siteId: site.id,
        file,
      }),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(toastSuccess).toHaveBeenCalledWith("Deployed site.tar.gz to docs");
  });

  it("surfaces a rejected upload and stays open", async () => {
    deploySite.mutateAsync.mockRejectedValue(
      new Error("site archive must contain a regular index.html at its root"),
    );
    const { onOpenChange } = renderDialog();
    chooseFile("site.zip");

    fireEvent.click(screen.getByRole("button", { name: "Deploy" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "site archive must contain a regular index.html at its root",
    );
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("forgets the chosen file when it is dismissed", () => {
    renderDialog();
    chooseFile("site.zip");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.getByRole("button", { name: "Deploy" })).toBeDisabled();
  });
});
