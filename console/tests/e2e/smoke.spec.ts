import { expect, test } from "@playwright/test";

test("login form is available without an API dependency", async ({ page }) => {
  await page.goto("/login");
  await expect(
    page.getByRole("heading", { name: "Sign in to Stealth" }),
  ).toBeVisible();
  await expect(page.getByLabel("Email", { exact: true })).toBeVisible();
  await expect(page.getByLabel("Password", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByText("Enter a valid email address.")).toBeVisible();
});

test("authentication links are discoverable", async ({ page }) => {
  await page.goto("/login");
  await expect(
    page.getByRole("link", { name: "email", exact: true }),
  ).toHaveAttribute("href", "/forgot-password");
  await expect(
    page.getByRole("link", { name: "password", exact: true }),
  ).toHaveAttribute("href", "/forgot-password");
  await expect(page.getByRole("link", { name: "Sign up" })).toHaveAttribute(
    "href",
    "/register",
  );
});

test("unsafe auth next paths fall back to the admin console", async ({
  page,
}) => {
  for (const unsafeNext of ["//evil.example", "https://evil.example"]) {
    await page.goto("/login?next=" + encodeURIComponent(unsafeNext));
    const registerHref = await page
      .getByRole("link", { name: "Sign up" })
      .getAttribute("href");
    expect(
      new URL(registerHref ?? "", "http://127.0.0.1").searchParams.get("next"),
    ).toBe("/admin");

    await page.goto("/register?next=" + encodeURIComponent(unsafeNext));
    const signInHref = await page
      .getByRole("link", { name: "Sign in" })
      .getAttribute("href");
    expect(
      new URL(signInHref ?? "", "http://127.0.0.1").searchParams.get("next"),
    ).toBe("/admin");
  }
});
