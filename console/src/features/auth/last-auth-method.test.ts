import { afterEach, describe, expect, it } from "vitest";
import {
  readLastAuthMethod,
  resetLastAuthMethodCache,
  writeLastAuthMethod,
} from "./last-auth-method";

// The "Last used" badge is driven by this device-local value. These cases pin
// that it round-trips, rejects unknown values, and fails closed on bad storage.
describe("last auth method storage", () => {
  afterEach(() => {
    window.localStorage.clear();
    // Reads are cached in memory, and clearing storage directly does not notify
    // the cache, so reset it between cases.
    resetLastAuthMethodCache();
  });

  it("round-trips a known method", () => {
    writeLastAuthMethod("google");
    expect(readLastAuthMethod()).toBe("google");
    writeLastAuthMethod("password");
    expect(readLastAuthMethod()).toBe("password");
  });

  it("ignores an unknown stored value", () => {
    window.localStorage.setItem("stealth.login.last-method:v1", "saml");
    expect(readLastAuthMethod()).toBeNull();
  });

  it("returns null when nothing is stored", () => {
    expect(readLastAuthMethod()).toBeNull();
  });
});
