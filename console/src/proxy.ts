import { NextResponse, type NextRequest } from "next/server";
import { consoleRedirectFor } from "@/features/auth/console-home";
import { resolveSessionCached } from "@/features/auth/session.server";

/**
 * Guards the two console surfaces before any HTML is sent.
 *
 * Without this, /admin is a static document that every visitor receives, and
 * the browser only discovers it has no session after the page loads: the
 * address bar says /admin, the screen is empty, and only then does a client
 * redirect fire. Non-admins additionally went /admin -> /user, so two wrong
 * URLs appeared before the right one.
 *
 * The API remains the authority; this only decides earlier where to send
 * someone. `consoleRedirectFor` holds the rule and is unit-tested.
 */
export async function proxy(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  // Cached (a few seconds per cookie): this runs on every request the matcher
  // sees, prefetches included, and the rule below is unchanged by reuse.
  const session = await resolveSessionCached(request.headers.get("cookie"));
  const destination = consoleRedirectFor(session, pathname, search);
  if (!destination) return NextResponse.next();

  const url = request.nextUrl.clone();
  const [path, query] = destination.split("?");
  url.pathname = path;
  url.search = query ? `?${query}` : "";
  return NextResponse.redirect(url);
}

export const config = {
  // Only the two guarded surfaces. Assets, the API proxy, and the auth pages
  // stay untouched, so this cannot block CSS, JS, or the sign-in flow.
  matcher: ["/admin", "/admin/:path*", "/user", "/user/:path*"],
};
