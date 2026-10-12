/**
 * Where a signed-in account belongs. Instance owners and admins operate the
 * Admin Console; every other account uses the customer console.
 *
 * Shared by the post-login redirect and the server-side check that keeps an
 * already-authenticated visitor off the sign-in and sign-up pages.
 */
export function consoleHomeForRole(role: string | null | undefined) {
  return role === "instance_owner" || role === "instance_admin"
    ? "/admin"
    : "/user";
}
