import { cookies } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";
import { consoleHomeForRole } from "@/features/auth/console-home";
import { resolveSession } from "@/features/auth/session.server";

// Reads the session cookie and calls the API, so this route is per-request.
export const dynamic = "force-dynamic";

/**
 * The console root has no surface of its own: it sends each visitor to the
 * console that matches their session.
 *
 * It resolves the session here, on the server, rather than redirecting everyone
 * to /admin and letting the browser discover it is signed out. That old shape
 * showed anonymous visitors an /admin URL and an empty screen before bouncing
 * them to /login.
 *
 * An unknown session (`unavailable`) is not treated as a signed-out one: the
 * same rule the proxy follows. Sending a possibly signed-in operator to /login
 * would strand them on a page that forwards authenticated visitors away, so
 * this page renders a retry note instead until the API answers. An anonymous
 * visitor — no cookie at all — still goes straight to /login, which is what the
 * ingress probes and smoke checks hold us to.
 */
export default async function HomePage() {
  const session = await resolveSession((await cookies()).toString());
  if (session.status === "authenticated") {
    redirect(consoleHomeForRole(session.role));
  }
  if (session.status === "unavailable") {
    return (
      <main className="flex min-h-dvh items-center justify-center px-6">
        <div className="text-center">
          <p className="text-[14px] font-medium text-[#f2f2f3]">
            Console is unreachable
          </p>
          <p className="mt-2 text-[13px] leading-5 text-[#6b6b72]">
            The platform API did not answer, so the Console cannot tell where to
            send you yet.
          </p>
          <Link
            href="/"
            className="mt-4 inline-block text-[13px] font-medium text-[#4d8dff] underline underline-offset-2 hover:text-[#7fabff]"
          >
            Retry
          </Link>
        </div>
      </main>
    );
  }
  redirect("/login");
}
