import "server-only";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { getSafeNextPath } from "@/lib/navigation";
import { consoleHomeForRole } from "./console-home";

// The setup API and the console share the internal alias `stealth-api` in both
// the production and setup Compose projects, so one default covers both.
const INTERNAL_API_URL = (
  process.env.STEALTH_API_INTERNAL_URL ?? "http://stealth-api:8080"
).replace(/\/+$/, "");

const PROBE_TIMEOUT_MS = 2_000;

type AccountResponse = { account?: { instance_role?: string | null } };

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
  const cookieHeader = (await cookies()).toString();
  if (!cookieHeader) return;

  let account: AccountResponse | null = null;
  try {
    const response = await fetch(`${INTERNAL_API_URL}/v1/account`, {
      headers: { cookie: cookieHeader },
      cache: "no-store",
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    });
    if (response.ok) account = (await response.json()) as AccountResponse;
  } catch {
    return;
  }
  if (!account) return;

  // An explicit ?next= wins, so invitations and deep links keep working. It is
  // validated here rather than trusted from the query string.
  const next = rawNext === null ? null : getSafeNextPath(rawNext);
  redirect(next ?? consoleHomeForRole(account.account?.instance_role));
}
