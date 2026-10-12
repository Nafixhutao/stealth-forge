import { describe, expect, it } from "vitest";
import { consoleHomeForRole } from "./console-home";

describe("consoleHomeForRole", () => {
  it("sends instance owners and admins to the admin console", () => {
    expect(consoleHomeForRole("instance_owner")).toBe("/admin");
    expect(consoleHomeForRole("instance_admin")).toBe("/admin");
  });

  it("sends every other role to the user console", () => {
    expect(consoleHomeForRole("member")).toBe("/user");
    expect(consoleHomeForRole("")).toBe("/user");
  });

  it("treats a missing role as the user console", () => {
    expect(consoleHomeForRole(undefined)).toBe("/user");
    expect(consoleHomeForRole(null)).toBe("/user");
  });
});
