import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SiteSource } from "@/features/project/site-source";

// The source panel is read-only, and that is the point: a release is immutable,
// so the panel must not pretend to save. It lists a release's files, opens the
// entry point first, and renders text while refusing to guess at binaries.
const useSiteDeploymentFiles = vi.fn();

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
  CodeViewer: ({ value, language }: { value: string; language: string }) => (
    <pre data-testid="code-viewer" data-language={language}>
      {value}
    </pre>
  ),
}));

vi.mock("next/dynamic", () => ({
  default: () =>
    function DynamicStub(props: { value: string; language: string }) {
      return (
        <pre data-testid="code-viewer" data-language={props.language}>
          {props.value}
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

function renderSource() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <SiteSource
        projectId="33333333-3333-3333-3333-333333333331"
        siteId={SITE_ID}
        deploymentId={DEPLOYMENT_ID}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.unstubAllGlobals();
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
