import { describe, expect, it } from "vitest";
import { formatConversationStamp, organizationDisplayName } from "@/lib/format";

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

describe("formatConversationStamp", () => {
  it("stamps today's turn with a dotted 24-hour time", () => {
    const now = new Date();
    now.setHours(18, 32, 0, 0);

    expect(formatConversationStamp(now.toISOString())).toBe("Today 18.32");
  });

  it("names the calendar day for an older turn", () => {
    const older = new Date();
    older.setDate(older.getDate() - 5);
    older.setHours(9, 5, 0, 0);

    expect(formatConversationStamp(older.toISOString())).toMatch(
      /^\w{3} \d{1,2} 09\.05$/,
    );
  });

  it("returns nothing for a missing or invalid stamp", () => {
    expect(formatConversationStamp(null)).toBe("");
    expect(formatConversationStamp("not-a-date")).toBe("");
  });
});
