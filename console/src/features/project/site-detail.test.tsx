import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SiteDetail } from "@/features/project/site-detail";

// The page's contract: what it previews, which releases it offers to act on, and
// that the source panel only appears for a release that actually has files.
const useSite = vi.fn();
const useSiteDeployments = vi.fn();
const useSiteDeploymentLogs = vi.fn();
const useSiteDeploymentFiles = vi.fn();
const activateDeployment = { mutate: vi.fn(), isPending: false };
const deleteDeployment = { mutate: vi.fn(), isPending: false };

vi.mock("@/api/queries/site-detail", () => ({
  useSite: (projectId: string, siteId: string) => useSite(projectId, siteId),
  useSiteDeployments: (projectId: string, siteId: string) =>
    useSiteDeployments(projectId, siteId),
  useSiteDeploymentLogs: (
    projectId: string,
    siteId: string,
    deploymentId: string,
    active: boolean,
  ) => useSiteDeploymentLogs(projectId, siteId, deploymentId, active),
  useSiteDeploymentFiles: (
    projectId: string,
    siteId: string,
    deploymentId: string,
    enabled: boolean,
  ) => useSiteDeploymentFiles(projectId, siteId, deploymentId, enabled),
}));

vi.mock("@/api/mutations/sites", () => ({
  useActivateSiteDeployment: () => activateDeployment,
  useDeleteSiteDeployment: () => deleteDeployment,
  useCreateGitDeployment: () => ({ mutateAsync: vi.fn(), isPending: false }),
}));

vi.mock("next/link", () => ({
  default: ({
    href,
    children,
    ...rest
  }: {
    href: string;
    children: React.ReactNode;
  }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// The source panel has its own tests; here it only needs to exist.
vi.mock("@/features/project/site-source", () => ({
  SiteSource: ({ deploymentId }: { deploymentId: string }) => (
    <div data-testid="site-source">{deploymentId}</div>
  ),
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";
const SITE_ID = "44444444-4444-4444-4444-444444444441";

function query(overrides: Record<string, unknown> = {}) {
  return {
    data: undefined,
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    hasNextPage: false,
    isFetchingNextPage: false,
    fetchNextPage: vi.fn(),
    ...overrides,
  };
}

function deployment(overrides: Record<string, unknown> = {}) {
  return {
    id: "dep-1",
    site_id: SITE_ID,
    project_id: PROJECT_ID,
    version: 1,
    source: "upload",
    source_name: "site.zip",
    size_bytes: 2048,
    archive_size_bytes: 1024,
    checksum_sha256: "a".repeat(64),
    status: "active",
    build_status: "succeeded",
    reserved_bytes: 0,
    activate_requested: true,
    queued_at: "2026-10-18T10:00:00.000Z",
    created_at: "2026-10-18T10:00:00.000Z",
    updated_at: "2026-10-18T10:00:00.000Z",
    ...overrides,
  };
}

function site(overrides: Record<string, unknown> = {}) {
  return {
    id: SITE_ID,
    project_id: PROJECT_ID,
    name: "docs",
    platform_hostname: "docs.cherome.my.id",
    framework: "static",
    enabled: true,
    status: "active",
    artifact_quota_bytes: 1073741824,
    artifact_used_bytes: 2048,
    artifact_reserved_bytes: 0,
    active_deployment_id: "dep-1",
    created_at: "2026-10-15T09:40:00.000Z",
    updated_at: "2026-10-17T12:00:00.000Z",
    ...overrides,
  };
}

function renderDetail() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <SiteDetail projectId={PROJECT_ID} siteId={SITE_ID} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useSite.mockReturnValue(query({ data: { site: site() } }));
  useSiteDeployments.mockReturnValue(
    query({
      data: {
        pages: [
          {
            deployments: [
              deployment(),
              deployment({
                id: "dep-2",
                version: 2,
                status: "superseded",
                build_status: "succeeded",
              }),
            ],
            can_manage: true,
          },
        ],
      },
    }),
  );
  useSiteDeploymentLogs.mockReturnValue(query({ data: { logs: [] } }));
  useSiteDeploymentFiles.mockReturnValue(
    query({ data: { files: [], truncated: false } }),
  );
});

describe("SiteDetail", () => {
  it("previews the live release in a sandboxed frame", () => {
    renderDetail();

    const frame = screen.getByTitle("Preview of docs");
    expect(frame).toHaveAttribute(
      "src",
      "/v1/sites/44444444-4444-4444-4444-444444444441?v=0",
    );
    // Without sandboxing, published site code could read the console session.
    expect(frame).toHaveAttribute("sandbox");
    expect(frame.getAttribute("sandbox")).not.toContain("allow-same-origin");
  });

  it("names the site, its address, and that it is live", () => {
    renderDetail();

    expect(screen.getByRole("heading", { name: "docs" })).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /docs\.cherome\.my\.id/ }),
    ).toHaveAttribute("href", "https://docs.cherome.my.id");
    expect(screen.getByText("Live")).toBeInTheDocument();
  });

  it("says there is nothing to preview when no release is live", () => {
    useSite.mockReturnValue(
      query({ data: { site: site({ active_deployment_id: null }) } }),
    );

    renderDetail();

    expect(screen.queryByTitle(/Preview of/)).toBeNull();
    expect(
      screen.getByText("Deploy a release to see the site here."),
    ).toBeInTheDocument();
    expect(screen.getByText("Not deployed")).toBeInTheDocument();
  });

  it("offers make-live and delete only for a release that is not live", () => {
    renderDetail();

    // The live release cannot be deleted; the superseded one can be promoted.
    expect(screen.getAllByRole("button", { name: "Make live" })).toHaveLength(
      1,
    );
    expect(
      screen.getByRole("button", { name: "Delete release 2" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Delete release 1" }),
    ).toBeNull();
  });

  it("activates the release the operator chose", () => {
    renderDetail();

    fireEvent.click(screen.getByRole("button", { name: "Make live" }));

    expect(activateDeployment.mutate).toHaveBeenCalledWith(
      { siteId: SITE_ID, deploymentId: "dep-2" },
      expect.anything(),
    );
  });

  it("shows the build log when asked, and only then", () => {
    useSiteDeploymentLogs.mockReturnValue(
      query({
        data: {
          logs: [
            {
              id: "l1",
              level: "info",
              message: "cloning repository",
              sequence: 1,
            },
            { id: "l2", level: "error", message: "build failed", sequence: 2 },
          ],
        },
      }),
    );

    renderDetail();

    expect(screen.queryByText("cloning repository")).toBeNull();
    fireEvent.click(screen.getAllByRole("button", { name: "Build log" })[0]);
    expect(screen.getByText("cloning repository")).toBeInTheDocument();
    expect(screen.getByText("build failed")).toBeInTheDocument();
  });

  it("opens the source of the live release by default", () => {
    renderDetail();

    expect(screen.getByTestId("site-source")).toHaveTextContent("dep-1");
    expect(screen.getByText("read-only")).toBeInTheDocument();
  });

  it("switches the source panel to the release whose row was chosen", () => {
    renderDetail();

    fireEvent.click(screen.getAllByRole("button", { name: "View source" })[1]);

    expect(screen.getByTestId("site-source")).toHaveTextContent("dep-2");
  });

  it("withholds the source panel until a release has published files", () => {
    useSiteDeployments.mockReturnValue(
      query({
        data: {
          pages: [
            {
              deployments: [
                deployment({
                  id: "dep-3",
                  version: 3,
                  status: "queued",
                  build_status: "running",
                  active_deployment_id: null,
                }),
              ],
              can_manage: true,
            },
          ],
        },
      }),
    );
    useSite.mockReturnValue(
      query({ data: { site: site({ active_deployment_id: null }) } }),
    );

    renderDetail();

    expect(screen.queryByTestId("site-source")).toBeNull();
    expect(
      screen.getByText(
        /The source of a release appears here once it has finished building/,
      ),
    ).toBeInTheDocument();
  });

  it("hides write controls from a caller who may only read", () => {
    useSiteDeployments.mockReturnValue(
      query({
        data: {
          pages: [
            {
              deployments: [
                deployment(),
                deployment({ id: "dep-2", version: 2 }),
              ],
              can_manage: false,
            },
          ],
        },
      }),
    );

    renderDetail();

    expect(
      screen.queryByRole("button", { name: "Deploy from Git" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "Make live" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Delete release/ })).toBeNull();
  });
});
