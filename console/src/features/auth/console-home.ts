/**
 * The single definition of "may operate the Admin Console". Instance owners and
 * admins operate the platform; every other account uses the customer console.
 *
 * Shared by the post-login redirect, the proxy that guards /admin before it
 * renders, the AdminGate that re-checks in the browser, and the server-side
 * check that keeps a signed-in visitor off the sign-in page.
 */
export function isInstanceAdminRole(role: string | null | undefined) {
  return role === "instance_owner" || role === "instance_admin";
}

/** Where a signed-in account belongs. */
export function consoleHomeForRole(role: string | null | undefined) {
  return isInstanceAdminRole(role) ? "/admin" : "/user";
}

/** True for the customer console surfaces the proxy guards. */
export function isGuardedConsolePath(pathname: string) {
  return (
    pathname === "/admin" ||
    pathname.startsWith("/admin/") ||
    pathname === "/user" ||
    pathname.startsWith("/user/") ||
    pathname === "/agent" ||
    pathname.startsWith("/agent/")
  );
}

/**
 * Decides whether a visitor may stay on `pathname`, and where to send them
 * instead if not. Pure, so the routing rule is testable without a server.
 *
 * `anonymous` returns `/login` with the destination preserved, so signing in
 * lands where the visitor was headed. `unavailable` (the API could not answer)
 * returns null: the session is unknown rather than absent, and redirecting on a
 * probe failure would strand a signed-in operator on the sign-in page, which
 * forwards authenticated visitors straight back.
 */
export function consoleRedirectFor(
  session:
    | { status: "authenticated"; role: string | null }
    | { status: "anonymous" }
    | { status: "unavailable" },
  pathname: string,
  search = "",
): string | null {
  if (session.status === "unavailable") return null;

  if (session.status === "anonymous") {
    const destination = isGuardedConsolePath(pathname)
      ? `?next=${encodeURIComponent(pathname + search)}`
      : "";
    return `/login${destination}`;
  }

  const admin = isInstanceAdminRole(session.role);
  const wantsAdmin = pathname === "/admin" || pathname.startsWith("/admin/");
  if (wantsAdmin === admin) return null;
  return admin ? "/admin" : "/user";
}
