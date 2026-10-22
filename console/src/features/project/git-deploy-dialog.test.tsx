import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { GitDeployDialog } from "@/features/project/git-deploy-dialog";

// The dialog is the console's half of "connect a repo". It must reject what the
// API rejects — non-HTTPS, non-GitHub/GitLab, and anything that is not a
// repository — before a build is queued, and it must send the build recipe.
const createGitDeployment = { mutateAsync: vi.fn(), isPending: false };
const toastSuccess = vi.hoisted(() => vi.fn());

vi.mock("@/api/mutations/sites", () => ({
  useCreateGitDeployment: () => createGitDeployment,
}));

vi.mock("sonner", () => ({ toast: { success: toastSuccess } }));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";
const SITE_ID = "44444444-4444-4444-4444-444444444441";

function renderDialog() {
  const onOpenChange = vi.fn();
  render(
    <GitDeployDialog
      projectId={PROJECT_ID}
      siteId={SITE_ID}
      siteName="docs"
      open
      onOpenChange={onOpenChange}
    />,
  );
  return { onOpenChange };
}

const submitButton = () =>
  screen.getByRole("button", { name: "Build and deploy" });

beforeEach(() => {
  vi.clearAllMocks();
  createGitDeployment.mutateAsync.mockResolvedValue({
    deployment: { id: "dep-2" },
  });
});

describe("GitDeployDialog", () => {
  it("says what it cannot do instead of implying a GitHub integration", () => {
    renderDialog();

    expect(
      screen.getByText(
        /private repositories and push-triggered rebuilds are not supported yet/i,
      ),
    ).toBeInTheDocument();
  });

  it("refuses a repository the API would reject", () => {
    renderDialog();

    const field = screen.getByLabelText("Repository");

    fireEvent.change(field, { target: { value: "http://github.com/o/r" } });
    expect(
      screen.getByText("The repository URL must use HTTPS."),
    ).toBeInTheDocument();
    expect(submitButton()).toBeDisabled();

    fireEvent.change(field, { target: { value: "https://example.com/o/r" } });
    expect(
      screen.getByText(
        "Only github.com and gitlab.com repositories are supported.",
      ),
    ).toBeInTheDocument();

    fireEvent.change(field, { target: { value: "https://github.com/owner" } });
    expect(screen.getByText(/must name a repository/)).toBeInTheDocument();

    fireEvent.change(field, {
      target: { value: "https://github.com/owner/repo" },
    });
    expect(submitButton()).toBeEnabled();
  });

  it("queues a build with the recipe it showed", async () => {
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText("Repository"), {
      target: { value: "https://github.com/owner/repo" },
    });
    fireEvent.change(screen.getByLabelText("Branch or tag"), {
      target: { value: "release" },
    });
    fireEvent.change(screen.getByLabelText("Runtime"), {
      target: { value: "python-3.13" },
    });
    fireEvent.change(screen.getByLabelText("Build command"), {
      target: { value: "make site" },
    });
    fireEvent.change(screen.getByLabelText("Output directory"), {
      target: { value: "build" },
    });
    fireEvent.click(submitButton());

    await waitFor(() =>
      expect(createGitDeployment.mutateAsync).toHaveBeenCalledWith({
        siteId: SITE_ID,
        body: {
          repository: "https://github.com/owner/repo",
          ref: "release",
          build_runtime: "python-3.13",
          build_command: "make site",
          output_directory: "build",
          activate: true,
        },
      }),
    );
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(toastSuccess).toHaveBeenCalledWith("Build queued for docs");
  });

  it("surfaces the API's reason and stays open", async () => {
    createGitDeployment.mutateAsync.mockRejectedValue(
      new Error("git provider archive unavailable"),
    );
    const { onOpenChange } = renderDialog();

    fireEvent.change(screen.getByLabelText("Repository"), {
      target: { value: "https://github.com/owner/repo" },
    });
    fireEvent.click(submitButton());

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "git provider archive unavailable",
    );
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});
