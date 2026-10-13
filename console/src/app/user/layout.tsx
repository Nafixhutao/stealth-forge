import { SessionGate } from "@/components/session-gate";

/** Every /user route requires an authenticated session. */
export default function UserLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return <SessionGate>{children}</SessionGate>;
}
