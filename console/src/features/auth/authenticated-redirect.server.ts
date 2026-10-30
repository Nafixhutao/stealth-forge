import "server-only";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { getSafeNextPath } from "@/lib/navigation";
import { consoleHomeForRole } from "./console-home";
import { resolveSession } from "./session.server";

/**
 * Keeps an already-authenticated visitor off a signed-out-only page.
 *
 * Runs on the server, before any HTML is sent, which matters twice: a
 * signed-in visitor never sees the form flash, and an anonymous visitor still
 * receives the form in the first response instead of waiting for a client-side
 * session check to resolve.
 *
 * Fails open: a missing cookie or an unreachable API leaves the form in place.
 */
export async function redirectIfAuthenticated(
  rawNext: string | null,
): Promise<void> {
  const session = await resolveSession((await cookies()).toString());
  if (session.status !== "authenticated") return;

  // An explicit ?next= wins, so invitations and deep links keep working. It is
  // validated here rather than trusted from the query string.
  const next = rawNext === null ? null : getSafeNextPath(rawNext);
  redirect(next ?? consoleHomeForRole(session.role));
}
