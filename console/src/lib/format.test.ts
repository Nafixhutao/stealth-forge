import { describe, expect, it } from "vitest";
import { organizationDisplayName } from "@/lib/format";

describe("organizationDisplayName", () => {
  it("drops the generated personal-organization suffix", () => {
    expect(organizationDisplayName("Aku Nafi's organization")).toBe("Aku Nafi");
    expect(organizationDisplayName("KURAMANAFI HOSSAIN’s organization")).toBe(
      "KURAMANAFI HOSSAIN",
    );
  });

  it("leaves a hand-named organization untouched", () => {
    expect(organizationDisplayName("Nafixhutao")).toBe("Nafixhutao");
    expect(organizationDisplayName("Acme Organization")).toBe(
      "Acme Organization",
    );
  });

  it("treats a blank name as no name so the caller can fall back", () => {
    expect(organizationDisplayName(undefined)).toBe("");
    expect(organizationDisplayName("   ")).toBe("");
  });
});
