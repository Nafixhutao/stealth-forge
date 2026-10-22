import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  useCreateDatabase,
  useDeleteDatabase,
} from "@/api/mutations/databases";
import { useCreateBucket, useDeleteBucket } from "@/api/mutations/storage";
import {
  useCreateFunction,
  useDeleteFunction,
} from "@/api/mutations/functions";
import { FunctionRuntime } from "@/api/generated/schema";

// These writes are six different endpoints that look alike, so the one thing
// worth pinning is the path each one hits and the ids it carries. A wrong id in
// a delete is silent until the wrong row disappears.
const postMock = vi.fn();
const deleteMock = vi.fn();

vi.mock("@/api/client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api/client")>();
  return {
    ...actual,
    api: {
      ...actual.api,
      POST: (...args: unknown[]) => postMock(...args),
      DELETE: (...args: unknown[]) => deleteMock(...args),
    },
  };
});

const PROJECT_ID = "33333333-3333-3333-3333-333333333331";

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  postMock.mockReset();
  deleteMock.mockReset();
  postMock.mockResolvedValue({
    data: {},
    response: new Response(null, { status: 201 }),
  });
  deleteMock.mockResolvedValue({
    data: undefined,
    response: new Response(null, { status: 204 }),
  });
});

describe("project resource writes", () => {
  it("creates a database under the project", async () => {
    const { result } = renderHook(() => useCreateDatabase(PROJECT_ID), {
      wrapper,
    });

    await result.current.mutateAsync({ name: "app" });

    const [path, init] = postMock.mock.calls[0] as [
      string,
      { params: { path: { projectID: string } }; body: unknown },
    ];
    expect(path).toBe("/v1/projects/{projectID}/databases");
    expect(init.params.path).toEqual({ projectID: PROJECT_ID });
    expect(init.body).toEqual({ name: "app" });
  });

  it("deletes the database it was given, not the project's first one", async () => {
    const { result } = renderHook(() => useDeleteDatabase(PROJECT_ID), {
      wrapper,
    });

    await result.current.mutateAsync("d2222222-2222-2222-2222-222222222222");

    const [path, init] = deleteMock.mock.calls[0] as [
      string,
      { params: { path: { projectID: string; databaseID: string } } },
    ];
    expect(path).toBe("/v1/projects/{projectID}/databases/{databaseID}");
    expect(init.params.path).toEqual({
      projectID: PROJECT_ID,
      databaseID: "d2222222-2222-2222-2222-222222222222",
    });
  });

  it("creates a bucket under the project's storage", async () => {
    const { result } = renderHook(() => useCreateBucket(PROJECT_ID), {
      wrapper,
    });

    await result.current.mutateAsync({ name: "avatars", file_security: true });

    const [path, init] = postMock.mock.calls[0] as [
      string,
      { params: { path: { projectID: string } } },
    ];
    expect(path).toBe("/v1/projects/{projectID}/storage/buckets");
    expect(init.params.path).toEqual({ projectID: PROJECT_ID });
  });

  it("deletes the bucket it was given", async () => {
    const { result } = renderHook(() => useDeleteBucket(PROJECT_ID), {
      wrapper,
    });

    await result.current.mutateAsync("b1111111-1111-1111-1111-111111111111");

    const [path, init] = deleteMock.mock.calls[0] as [
      string,
      { params: { path: { projectID: string; bucketID: string } } },
    ];
    expect(path).toBe("/v1/projects/{projectID}/storage/buckets/{bucketID}");
    expect(init.params.path.bucketID).toBe(
      "b1111111-1111-1111-1111-111111111111",
    );
  });

  it("creates a function with the metadata it was given", async () => {
    const { result } = renderHook(() => useCreateFunction(PROJECT_ID), {
      wrapper,
    });

    const body = {
      name: "send-receipt",
      runtime: FunctionRuntime.node_22,
      entrypoint: "src/main.js",
      commands: "",
      timeout_seconds: 15,
      enabled: true,
      logging: true,
    };
    await result.current.mutateAsync(body);

    const [path, init] = postMock.mock.calls[0] as [
      string,
      { params: { path: { projectID: string } }; body: unknown },
    ];
    expect(path).toBe("/v1/projects/{projectID}/functions");
    expect(init.body).toEqual(body);
  });

  it("deletes the function it was given", async () => {
    const { result } = renderHook(() => useDeleteFunction(PROJECT_ID), {
      wrapper,
    });

    await result.current.mutateAsync("f1111111-1111-1111-1111-111111111111");

    const [path, init] = deleteMock.mock.calls[0] as [
      string,
      { params: { path: { projectID: string; functionID: string } } },
    ];
    expect(path).toBe("/v1/projects/{projectID}/functions/{functionID}");
    expect(init.params.path.functionID).toBe(
      "f1111111-1111-1111-1111-111111111111",
    );
  });

  it("surfaces a rejected delete instead of reporting success", async () => {
    deleteMock.mockResolvedValue({
      error: {
        error: { code: "forbidden", message: "you do not have permission" },
      },
      response: new Response(null, { status: 403 }),
    });
    const { result } = renderHook(() => useDeleteDatabase(PROJECT_ID), {
      wrapper,
    });

    await expect(result.current.mutateAsync("d1")).rejects.toThrow(
      "you do not have permission",
    );
  });
});
