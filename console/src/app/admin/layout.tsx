import { AdminGate } from "@/features/admin/components/admin-gate";
import { AdminShell } from "@/features/admin/components/admin-shell";

/** All /admin routes share the admin-only chrome (separate from customer nav). */
export default function AdminLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <AdminGate>
      <AdminShell>{children}</AdminShell>
    </AdminGate>
  );
}
