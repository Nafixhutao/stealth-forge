import { SessionGate } from "@/components/session-gate";

/** Every /agent route needs an authenticated non-admin session; the proxy
 * keeps admins on /admin, so this gate only has to catch a mid-session 401. */
export default function AgentLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return <SessionGate>{children}</SessionGate>;
}
