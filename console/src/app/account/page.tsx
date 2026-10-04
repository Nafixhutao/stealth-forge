import type { Metadata } from "next";
import { AccountView } from "@/features/account/account-view";
import { ConsoleShell } from "@/components/layout/console-shell";

export const metadata: Metadata = {
  title: "Account",
  description: "Manage your Stealth Console account.",
};

export default function AccountPage() {
  return (
    <ConsoleShell>
      <AccountView />
    </ConsoleShell>
  );
}
