import { describe, expect, it } from "vitest";
import {
  consoleHomeForRole,
  consoleRedirectFor,
  isGuardedConsolePath,
  isInstanceAdminRole,
} from "./console-home";

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

describe("isInstanceAdminRole", () => {
  it("is true only for the two instance roles", () => {
    expect(isInstanceAdminRole("instance_owner")).toBe(true);
    expect(isInstanceAdminRole("instance_admin")).toBe(true);
    expect(isInstanceAdminRole("member")).toBe(false);
    expect(isInstanceAdminRole(null)).toBe(false);
  });
});

describe("isGuardedConsolePath", () => {
  it("matches the guarded customer surfaces and nothing else", () => {
    expect(isGuardedConsolePath("/admin")).toBe(true);
    expect(isGuardedConsolePath("/admin/logs")).toBe(true);
    expect(isGuardedConsolePath("/user")).toBe(true);
    expect(isGuardedConsolePath("/user/settings")).toBe(true);
    expect(isGuardedConsolePath("/agent")).toBe(true);
    expect(isGuardedConsolePath("/agent/thread")).toBe(true);
    expect(isGuardedConsolePath("/login")).toBe(false);
    expect(isGuardedConsolePath("/administrator")).toBe(false);
    expect(isGuardedConsolePath("/agents")).toBe(false);
  });
});

describe("consoleRedirectFor", () => {
  const admin = { status: "authenticated", role: "instance_owner" } as const;
  const member = { status: "authenticated", role: "member" } as const;
  const anonymous = { status: "anonymous" } as const;
  const unavailable = { status: "unavailable" } as const;

  it("sends an anonymous visitor to sign-in and keeps the destination", () => {
    expect(consoleRedirectFor(anonymous, "/admin/logs", "?range=1h")).toBe(
      "/login?next=%2Fadmin%2Flogs%3Frange%3D1h",
    );
  });

  it("does not add a next parameter for an unguarded path", () => {
    expect(consoleRedirectFor(anonymous, "/")).toBe("/login");
  });

  it("lets an admin stay on the admin console", () => {
    expect(consoleRedirectFor(admin, "/admin")).toBeNull();
    expect(consoleRedirectFor(admin, "/admin/logs")).toBeNull();
  });

  it("moves a non-admin off the admin console", () => {
    expect(consoleRedirectFor(member, "/admin")).toBe("/user");
    expect(consoleRedirectFor(member, "/admin/logs")).toBe("/user");
  });

  it("moves an admin off the customer console", () => {
    expect(consoleRedirectFor(admin, "/user")).toBe("/admin");
  });

  it("lets a non-admin stay on the customer console", () => {
    expect(consoleRedirectFor(member, "/user")).toBeNull();
  });

  // An outage is not evidence of a signed-out visitor. Redirecting would send a
  // signed-in operator to /login, which forwards authenticated visitors back,
  // so the two would loop.
  it("never redirects when the session is unknown", () => {
    expect(consoleRedirectFor(unavailable, "/admin")).toBeNull();
    expect(consoleRedirectFor(unavailable, "/user")).toBeNull();
  });
});
