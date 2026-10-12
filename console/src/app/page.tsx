import { redirect } from "next/navigation";

/**
 * The console root has no surface of its own. It hands off to the Admin
 * Console deterministically (a real 307, which ingress acceptance checks
 * expect); AdminGate forwards signed-in non-admins to the user console at
 * /user, and unauthenticated visitors to /login.
 */
export default function HomePage() {
  redirect("/admin");
}
