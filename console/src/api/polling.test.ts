import { describe, expect, it } from "vitest";
import {
  POLL_FAST_MS,
  POLL_MEDIUM_MS,
  POLL_SLOW_MS,
  polledQuery,
} from "@/api/polling";

describe("polledQuery", () => {
  it("derives staleTime from the caller interval", () => {
    expect(polledQuery(5_000)).toEqual({
      refetchInterval: 5_000,
      staleTime: 5_000,
    });
  });

  it("derives staleTime from the fallback when the caller is silent", () => {
    expect(polledQuery(undefined, POLL_FAST_MS)).toEqual({
      refetchInterval: POLL_FAST_MS,
      staleTime: POLL_FAST_MS,
    });
  });

  it("lets the caller override the fallback", () => {
    expect(polledQuery(60_000, POLL_FAST_MS)).toEqual({
      refetchInterval: 60_000,
      staleTime: 60_000,
    });
  });

  it("omits staleTime when polling is disabled, so the global default applies", () => {
    expect(polledQuery(false, POLL_FAST_MS)).toEqual({
      refetchInterval: false,
    });
    expect(polledQuery(false, POLL_FAST_MS)).not.toHaveProperty("staleTime");
  });

  it("omits staleTime when no interval applies at all", () => {
    expect(polledQuery(undefined)).toEqual({ refetchInterval: undefined });
    expect(polledQuery(undefined)).not.toHaveProperty("staleTime");
  });

  it("never treats a polled query as fresh longer than its own cadence", () => {
    for (const interval of [POLL_FAST_MS, POLL_MEDIUM_MS, POLL_SLOW_MS]) {
      expect(polledQuery(interval).staleTime).toBeLessThanOrEqual(interval);
    }
  });
});
