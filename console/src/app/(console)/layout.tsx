import type { Metadata } from "next";
import { ConsoleShell } from "@/components/layout/console-shell";

export const metadata: Metadata = {
  title: "Console",
  description: "Stealth Console projects, services, and operations.",
};

export default function ConsoleLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return <ConsoleShell>{children}</ConsoleShell>;
}
