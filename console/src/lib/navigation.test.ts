import { describe, expect, it } from "vitest";
import { getSafeNextPath } from "@/lib/navigation";

describe("safe next paths", () => {
  it("keeps internal paths and rejects external destinations", () => {
    expect(getSafeNextPath("/admin/users")).toBe("/admin/users");
    expect(getSafeNextPath("//evil.example")).toBe("/admin");
    expect(getSafeNextPath("https://evil.example")).toBe("/admin");
    expect(getSafeNextPath(undefined)).toBe("/admin");
  });
});
