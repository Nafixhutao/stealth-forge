import { describe, expect, it } from "vitest";
import { ApiError } from "@/api/client";
import { errorMessage } from "./error-state";

describe("errorMessage", () => {
  it("distinguishes rejected sign-in credentials from an expired session", () => {
    expect(
      errorMessage(
        new ApiError("invalid email or password", 401, "invalid_credentials"),
      ),
    ).toBe(
      "That email and password do not match. If this account uses Google sign-in, choose Google above.",
    );
  });

  it("keeps the expired-session copy for an unauthorized account read", () => {
    expect(
      errorMessage(
        new ApiError("authentication is required", 401, "unauthorized"),
      ),
    ).toBe("Your session has expired. Sign in again to continue.");
  });
});
