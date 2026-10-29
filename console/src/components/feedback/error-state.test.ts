import { describe, expect, it } from "vitest";
import { ApiError } from "@/api/client";
import { errorMessage, isCredentialRejection } from "./error-state";

describe("errorMessage", () => {
  it("distinguishes rejected sign-in credentials from an expired session", () => {
    expect(
      errorMessage(
        new ApiError("invalid email or password", 401, "invalid_credentials"),
      ),
    ).toBe("That email and password do not match.");
  });

  // This mapper is shared by sign-in, email verification, and the Admin
  // provider settings. Naming a provider here told a GitHub-linked account to
  // "choose Google above" on screens that have no provider buttons at all.
  it("never names a sign-in provider", () => {
    const message = errorMessage(
      new ApiError("invalid email or password", 401, "invalid_credentials"),
    );
    for (const provider of ["Google", "GitHub"]) {
      expect(message).not.toContain(provider);
    }
  });

  it("keeps the expired-session copy for an unauthorized account read", () => {
    expect(
      errorMessage(
        new ApiError("authentication is required", 401, "unauthorized"),
      ),
    ).toBe("Your session has expired. Sign in again to continue.");
  });
});

describe("isCredentialRejection", () => {
  it("is true only for a rejected credential", () => {
    expect(
      isCredentialRejection(
        new ApiError("invalid email or password", 401, "invalid_credentials"),
      ),
    ).toBe(true);
    expect(
      isCredentialRejection(
        new ApiError("authentication is required", 401, "unauthorized"),
      ),
    ).toBe(false);
    expect(isCredentialRejection(new Error("boom"))).toBe(false);
    expect(isCredentialRejection(null)).toBe(false);
  });
});
