import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SiteSource } from "@/features/project/site-source";

// The source panel is read-only, and that is the point: a release is immutable,
// so the panel must not pretend to save. It lists a release's files, opens the
// entry point first, and renders text while refusing to guess at binaries.
const useSiteDeploymentFiles = vi.fn();
const editFile = { mutate: vi.fn(), isPending: false };
const toastSuccess = vi.hoisted(() => vi.fn());

vi.mock("@/api/mutations/sites", () => ({
  useEditSiteDeploymentFile: () => editFile,
}));

vi.mock("sonner", () => ({ toast: { success: toastSuccess, error: vi.fn() } }));

vi.mock("@/api/queries/site-detail", () => ({
  useSiteDeploymentFiles: (
    projectId: string,
    siteId: string,
    deploymentId: string,
    enabled: boolean,
  ) => useSiteDeploymentFiles(projectId, siteId, deploymentId, enabled),
}));

// The editor needs real layout, which jsdom does not have; the panel's contract
// is which file it asks for, so the viewer is replaced by its input.
vi.mock("@/features/project/code-viewer", () => ({
  languageForPath: (path: string) =>
    path.endsWith(".html") ? "html" : path.endsWith(".css") ? "css" : "text",
  CodeViewer: ({
    initialValue,
    language,
  }: {
    initialValue: string;
    language: string;
  }) => (
    <pre data-testid="code-viewer" data-language={language}>
      {initialValue}
    </pre>
  ),
}));

vi.mock("next/dynamic", () => ({
  default: () =>
    function DynamicStub(props: {
      initialValue: string;
      language: string;
      readOnly?: boolean;
      onChange?: (value: string) => void;
    }) {
      // Editable mode is a textarea so a test can type into it; read-only mode
      // stays a pre so its text is easy to assert.
      if (props.readOnly === false) {
        return (
          <textarea
            data-testid="code-editor"
            defaultValue={props.initialValue}
            onChange={(event) => props.onChange?.(event.target.value)}
          />
        );
      }
      return (
        <pre data-testid="code-viewer" data-language={props.language}>
          {props.initialValue}
        </pre>
      );
    },
}));

const SITE_ID = "44444444-4444-4444-4444-444444444441";
const DEPLOYMENT_ID = "55555555-5555-5555-5555-555555555551";

function query(overrides: Record<string, unknown> = {}) {
  return {
    data: undefined,
    isPending: false,
    isError: false,
    error: null,
    isFetching: false,
    ...overrides,
  };
}

function renderSource(props: { canEdit?: boolean; onPublished?: (d: unknown) => void } = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <SiteSource
        projectId="33333333-3333-3333-3333-333333333331"
        siteId={SITE_ID}
        deploymentId={DEPLOYMENT_ID}
        canEdit={props.canEdit}
        onPublished={props.onPublished as never}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
  editFile.mutate.mockReset();
  useSiteDeploymentFiles.mockReturnValue(
    query({
      data: {
        files: [
          { path: "index.html", size: 32 },
          { path: "assets/app.css", size: 16 },
          { path: "assets/logo.png", size: 128 },
        ],
        truncated: false,
      },
    }),
  );
});

describe("SiteSource", () => {
  it("lists the release's files with their sizes", () => {
    renderSource();

    expect(screen.getByText("index.html")).toBeInTheDocument();
    expect(screen.getByText("assets/app.css")).toBeInTheDocument();
    expect(screen.getByText("assets/logo.png")).toBeInTheDocument();
    expect(screen.getByText("32 B")).toBeInTheDocument();
  });

  it("opens the entry point first", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => "<!doctype html><title>docs</title>",
    });
    vi.stubGlobal("fetch", fetchMock);

    renderSource();

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining(
          `/v1/sites/${SITE_ID}/deployments/${DEPLOYMENT_ID}/index.html`,
        ),
        expect.anything(),
      ),
    );
    expect(await screen.findByTestId("code-viewer")).toHaveTextContent(
      "<!doctype html>",
    );
  });

  it("fetches a file through the release's public preview route", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => "body{}",
    });
    vi.stubGlobal("fetch", fetchMock);

    renderSource();
    fireEvent.click(screen.getByText("assets/app.css"));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        expect.stringContaining(
          `/v1/sites/${SITE_ID}/deployments/${DEPLOYMENT_ID}/assets/app.css`,
        ),
        expect.anything(),
      ),
    );
  });

  it("does not try to render a binary file as text", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      text: async () => "<!doctype html>",
    });
    vi.stubGlobal("fetch", fetchMock);

    renderSource();
    // The entry point loads first; the assertion below is about the image.
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    fireEvent.click(screen.getByText("assets/logo.png"));

    expect(
      screen.getByText(/This file is not text, so it is not shown here/),
    ).toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some((call) => String(call[0]).includes("logo.png")),
    ).toBe(false);
  });

  it("surfaces a file the release cannot serve", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: false, status: 404 }),
    );

    renderSource();

    expect(
      await screen.findByText("This file answered 404."),
    ).toBeInTheDocument();
  });

  it("offers no edit control to a reader", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, text: async () => "<html>" }),
    );

    renderSource();

    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("warns that saving publishes a new release before it is used", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, text: async () => "<html>" }),
    );

    renderSource({ canEdit: true });
    // The control is disabled until the file has been read.
    await screen.findByTestId("code-viewer");
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    expect(
      screen.getByText(/Saving publishes a/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/counts against the site's artifact quota/),
    ).toBeInTheDocument();
  });

  it("publishes the edited file and reports the new release", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, text: async () => "<html>old</html>" }),
    );
    const onPublished = vi.fn();
    editFile.mutate.mockImplementation(
      (_body: unknown, options: { onSuccess: (result: unknown) => void }) => {
        options.onSuccess({ deployment: { id: "dep-new", version: 3 } });
      },
    );

    renderSource({ canEdit: true, onPublished });
    await screen.findByTestId("code-viewer");
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByTestId("code-editor"), {
      target: { value: "<html>new</html>" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Publish changes" }));

    expect(editFile.mutate).toHaveBeenCalledWith(
      {
        siteId: SITE_ID,
        deploymentId: DEPLOYMENT_ID,
        path: "index.html",
        content: "<html>new</html>",
      },
      expect.anything(),
    );
    expect(onPublished).toHaveBeenCalledWith({ id: "dep-new", version: 3 });
    expect(toastSuccess).toHaveBeenCalled();
  });

  it("keeps Publish disabled until something changed", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, text: async () => "<html>" }),
    );

    renderSource({ canEdit: true });
    await screen.findByTestId("code-viewer");
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));

    expect(screen.getByRole("button", { name: "Publish changes" })).toBeDisabled();
    fireEvent.change(screen.getByTestId("code-editor"), {
      target: { value: "<html>changed</html>" },
    });
    expect(screen.getByRole("button", { name: "Publish changes" })).toBeEnabled();
  });

  it("drops the draft when the edit is cancelled", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, text: async () => "<html>old</html>" }),
    );

    renderSource({ canEdit: true });
    await screen.findByTestId("code-viewer");
    fireEvent.click(screen.getByRole("button", { name: "Edit" }));
    fireEvent.change(screen.getByTestId("code-editor"), {
      target: { value: "<html>scratch</html>" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByTestId("code-editor")).toBeNull();
    expect(screen.getByTestId("code-viewer")).toHaveTextContent("<html>old</html>");
    expect(editFile.mutate).not.toHaveBeenCalled();
  });

  it("says the release has no files instead of showing an empty tree", () => {
    useSiteDeploymentFiles.mockReturnValue(
      query({ isError: true, error: new Error("not_ready") }),
    );

    renderSource();

    expect(
      screen.getByText("This release has no published files yet."),
    ).toBeInTheDocument();
  });
});
