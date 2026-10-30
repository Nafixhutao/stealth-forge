import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resolveSession, resolveSessionCached } from "./session.server";

type ResponseStub = {
  ok: boolean;
  status: number;
  json: () => Promise<unknown>;
};

function stubFetch(
  implementation: (
    input: Parameters<typeof fetch>[0],
    init?: Parameters<typeof fetch>[1],
  ) => Promise<ResponseStub>,
) {
  const fetchMock = vi.fn(implementation as unknown as typeof fetch);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function jsonResponse(status: number, body: unknown): ResponseStub {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  };
}

describe("resolveSession", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("answers anonymous without calling the API when no cookie is present", async () => {
    const fetchMock = stubFetch(async () => {
      throw new Error("must not be called");
    });

    await expect(resolveSession(null)).resolves.toEqual({
      status: "anonymous",
    });
    await expect(resolveSession("")).resolves.toEqual({ status: "anonymous" });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("maps an unreachable API to unavailable", async () => {
    stubFetch(async () => {
      throw new TypeError("fetch failed");
    });

    await expect(resolveSession("session=abc")).resolves.toEqual({
      status: "unavailable",
    });
  });

  it("maps a 401 to anonymous", async () => {
    stubFetch(async () => jsonResponse(401, { error: "unauthenticated" }));

    await expect(resolveSession("session=abc")).resolves.toEqual({
      status: "anonymous",
    });
  });

  it("maps any other error status to unavailable", async () => {
    stubFetch(async () => jsonResponse(500, { error: "internal" }));

    await expect(resolveSession("session=abc")).resolves.toEqual({
      status: "unavailable",
    });
  });

  it("maps a 200 account body to authenticated with the instance role", async () => {
    stubFetch(async () =>
      jsonResponse(200, {
        account: { instance_role: "instance_owner" },
      }),
    );

    await expect(resolveSession("session=abc")).resolves.toEqual({
      status: "authenticated",
      role: "instance_owner",
    });
  });

  it("maps a 200 body without a role to authenticated with a null role", async () => {
    stubFetch(async () => jsonResponse(200, {}));

    await expect(resolveSession("session=abc")).resolves.toEqual({
      status: "authenticated",
      role: null,
    });
  });

  it("maps a 200 body that fails to parse as JSON to unavailable", async () => {
    stubFetch(async () => ({
      ok: true,
      status: 200,
      json: () => Promise.reject(new Error("invalid json")),
    }));

    await expect(resolveSession("session=abc")).resolves.toEqual({
      status: "unavailable",
    });
  });
});

describe("resolveSessionCached", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("reuses one resolution across the TTL window", async () => {
    let calls = 0;
    const fetchMock = stubFetch(async () => {
      calls += 1;
      return jsonResponse(200, { account: { instance_role: "member" } });
    });

    await expect(resolveSessionCached("session=burst")).resolves.toEqual({
      status: "authenticated",
      role: "member",
    });
    await expect(resolveSessionCached("session=burst")).resolves.toEqual({
      status: "authenticated",
      role: "member",
    });
    expect(calls).toBe(1);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("keys on the cookie header so visitors never share a result", async () => {
    stubFetch(async () => jsonResponse(200, {}));

    await resolveSessionCached("session=first-visitor");
    await resolveSessionCached("session=other-visitor");
    expect(vi.mocked(fetch).mock.calls.length).toBe(2);
  });

  it("resolves fresh after the TTL expires", async () => {
    let role = "member";
    stubFetch(async () =>
      jsonResponse(200, { account: { instance_role: role } }),
    );

    await expect(resolveSessionCached("session=abc")).resolves.toEqual({
      status: "authenticated",
      role: "member",
    });

    role = "instance_admin";
    await vi.advanceTimersByTimeAsync(5_001);
    await expect(resolveSessionCached("session=abc")).resolves.toEqual({
      status: "authenticated",
      role: "instance_admin",
    });
  });

  it("evicts the oldest entry past the cap", async () => {
    // Fill the cache (limit 1000) plus one more; the first key must then fall
    // out and resolve through the API again.
    const fetchedCookies: Array<string | undefined> = [];
    stubFetch(async (_input, init) => {
      fetchedCookies.push(
        (init?.headers as { cookie?: string } | undefined)?.cookie,
      );
      return jsonResponse(200, {});
    });

    await resolveSessionCached("session=evict-me");
    for (let index = 0; index < 1_000; index += 1) {
      await resolveSessionCached(`session=filler-${index}`);
    }
    const aliceFetches = () =>
      fetchedCookies.filter((cookie) => cookie === "session=evict-me").length;
    expect(aliceFetches()).toBe(1);

    await resolveSessionCached("session=evict-me");
    expect(aliceFetches()).toBe(2);
  });
});
