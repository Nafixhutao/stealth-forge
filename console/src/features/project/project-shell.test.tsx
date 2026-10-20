import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/api/client";
import { ProjectShell } from "@/features/project/project-shell";

// The frame is verified through its own contract: the project's identity, the
// section tabs it offers, and the one not-found answer it gives the sections
// below it. The project read and the account read are stubbed; the chrome the
// shell composes is stubbed too, so a failure here is this frame's.
const useProject = vi.fn();
const useOrganizations = vi.fn();
const useCurrentAccount = vi.fn();

vi.mock("@/api/queries/projects", () => ({
  useProject: (id: string | undefined) => useProject(id),
}));

vi.mock("@/api/queries/organizations", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/api/queries/organizations")>();
  return {
    // The frame flattens the pages with the real helper.
    organizationItems: actual.organizationItems,
    useOrganizations: () => useOrganizations(),
  };
});

vi.mock("@/api/queries/account", () => ({
  useCurrentAccount: () => useCurrentAccount(),
}));

vi.mock("@/features/user/user-top-bar", () => ({
  UserTopBar: () => <nav aria-label="Account bar" />,
}));

vi.mock("@/features/user/user-footer", () => ({
  UserFooter: () => <footer aria-label="Page footer" />,
}));

vi.mock("@/features/user/project-card", () => ({
  ProjectMark: () => <span data-testid="project-mark" />,
}));

let pathname = "/projects/33333333-3333-3333-3333-333333333331";

vi.mock("next/navigation", () => ({
  usePathname: () => pathname,
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), prefetch: vi.fn() }),
}));

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

type QueryStub = {
  data?: unknown;
  isPending: boolean;
  isError: boolean;
  error: unknown;
  refetch: () => void;
};

function query(overrides: Partial<QueryStub> = {}): QueryStub {
  return {
    data: undefined,
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...overrides,
  };
}

function renderShell(children = <p>section body</p>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <ProjectShell projectId={PROJECT_ID}>{children}</ProjectShell>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  pathname = `/projects/${PROJECT_ID}`;
  useCurrentAccount.mockReturnValue(
    query({
      data: {
        account: {
          id: "acc-1",
          email: "member@example.com",
          display_name: "Member",
          email_verified: true,
          instance_role: null,
          created_at: "2026-01-01T00:00:00Z",
        },
      },
    }),
  );
  useOrganizations.mockReturnValue(
    query({
      data: {
        pages: [
          {
            organizations: [
              {
                id: "11111111-1111-1111-1111-111111111111",
                name: "Acme Inc",
                slug: "acme",
                created_at: "2026-03-12T09:30:00Z",
              },
            ],
            pagination: { limit: 50, next_cursor: null },
          },
        ],
      },
    }),
  );
  useProject.mockReturnValue(
    query({
      data: {
        project: {
          id: PROJECT_ID,
          organization_id: "11111111-1111-1111-1111-111111111111",
          name: "checkout-api",
          created_at: "2026-10-15T09:12:00.000Z",
        },
      },
    }),
  );
});

describe("ProjectShell", () => {
  it("names the project and the organization it came from", () => {
    renderShell();

    expect(
      screen.getByRole("heading", { name: "checkout-api", level: 1 }),
    ).toBeInTheDocument();
    // The way back out is the organization the project belongs to, not a
    // generic "back" the user cannot predict.
    expect(screen.getByRole("link", { name: /Acme Inc/ })).toHaveAttribute(
      "href",
      "/user",
    );
  });

  it("offers every section as a real tab", () => {
    renderShell();

    const tabs = ["Overview", "Databases", "Storage", "Functions", "Sites"];
    for (const label of tabs) {
      expect(screen.getByRole("link", { name: label })).toBeInTheDocument();
    }
    expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute(
      "href",
      `/projects/${PROJECT_ID}`,
    );
    expect(screen.getByRole("link", { name: "Sites" })).toHaveAttribute(
      "href",
      `/projects/${PROJECT_ID}/sites`,
    );
  });

  it("marks the current section", () => {
    renderShell();

    expect(screen.getByRole("link", { name: "Overview" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("keeps a detail route's section selected", () => {
    // A detail page keeps its section selected, so the tabs stay a map of
    // where the user is rather than resetting on every drill-down.
    pathname = `/projects/${PROJECT_ID}/databases/db-1`;
    renderShell();

    expect(screen.getByRole("link", { name: "Databases" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Overview" })).not.toHaveAttribute(
      "aria-current",
    );
  });

  it("copies the project id from the header", () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.assign(navigator, { clipboard: { writeText } });

    renderShell();
    fireEvent.click(screen.getByRole("button", { name: "Copy project ID" }));

    expect(writeText).toHaveBeenCalledWith(PROJECT_ID);
  });

  it("answers a missing project once, without mounting the sections", () => {
    useProject.mockReturnValue(
      query({
        isError: true,
        error: new ApiError("project not found", 404),
      }),
    );

    renderShell();

    expect(screen.getByText("Project not found")).toBeInTheDocument();
    // The sections are not mounted for a project that does not resolve, so no
    // tab can fire a request for it; and there is nothing to retry.
    expect(screen.queryByText("section body")).toBeNull();
    expect(screen.queryByRole("link", { name: "Databases" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Retry/ })).toBeNull();
    // The header keeps the id the URL asked for instead of pulsing a
    // placeholder forever.
    expect(screen.getByTitle(PROJECT_ID)).toBeInTheDocument();
  });

  it("surfaces any other project failure with a retry", () => {
    const refetch = vi.fn();
    useProject.mockReturnValue(
      query({ isError: true, error: new ApiError("boom", 500), refetch }),
    );

    renderShell();

    expect(screen.getByText("Unable to load this project")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Retry/ }));
    expect(refetch).toHaveBeenCalled();
  });

  it("renders the section it is given, inside the frame", () => {
    renderShell(<p>section body</p>);

    expect(screen.getByText("section body")).toBeInTheDocument();
  });
});
