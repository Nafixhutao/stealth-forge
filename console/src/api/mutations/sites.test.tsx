import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useDeploySite } from "@/api/mutations/sites";

// The upload is the one write on this surface whose request shape is not
// obvious from the generated types: the schema types the archive as a string,
// while the browser sends it inside multipart FormData. This test pins the
// parts the API actually reads — one `source` file part and one `activate`
// field — by intercepting the typed operation before it reaches fetch.
const postMock = vi.fn();

vi.mock("@/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/client")>();
  return {
    ...actual,
    api: { ...actual.api, POST: (...args: unknown[]) => postMock(...args) },
  };
});

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";
const SITE_ID = "44444444-4444-4444-4444-444444444441";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  postMock.mockReset();
  postMock.mockResolvedValue({
    data: { deployment: { id: "dep-1" } },
    response: new Response(null, { status: 201 }),
  });
});

describe("useDeploySite", () => {
  it("sends the archive as multipart and asks for it to go live", async () => {
    const { result } = renderHook(() => useDeploySite(PROJECT_ID), {
      wrapper,
    });
    const file = new File(["archive"], "site.zip", {
      type: "application/zip",
    });

    await result.current.mutateAsync({ siteId: SITE_ID, file });

    expect(postMock).toHaveBeenCalledTimes(1);
    const [path, init] = postMock.mock.calls[0] as [
      string,
      {
        params: { path: { projectID: string; siteID: string } };
        body: FormData;
      },
    ];
    expect(path).toBe("/v1/projects/{projectID}/sites/{siteID}/deployments");
    expect(init.params.path).toEqual({
      projectID: PROJECT_ID,
      siteID: SITE_ID,
    });
    expect(init.body).toBeInstanceOf(FormData);
    expect(init.body.get("activate")).toBe("true");
    const source = init.body.get("source");
    expect(source).toBeInstanceOf(File);
    expect((source as File).name).toBe("site.zip");
    expect((source as File).size).toBe(file.size);
  });

  it("surfaces the API's rejection instead of reporting a deployment", async () => {
    postMock.mockResolvedValue({
      error: {
        error: {
          code: "validation_error",
          message: "site archive must contain a regular index.html at its root",
        },
      },
      response: new Response(null, { status: 422 }),
    });
    const { result } = renderHook(() => useDeploySite(PROJECT_ID), {
      wrapper,
    });

    await expect(
      result.current.mutateAsync({
        siteId: SITE_ID,
        file: new File(["archive"], "site.zip"),
      }),
    ).rejects.toThrow(
      "site archive must contain a regular index.html at its root",
    );
  });
});
