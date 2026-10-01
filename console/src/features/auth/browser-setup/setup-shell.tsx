import Link from "next/link";
import type { ReactNode } from "react";
import { setupSteps, type SetupStep } from "../browser-setup-model";
import { BrandMark } from "./primitives";

export function SetupShell({
  children,
  currentStep,
}: {
  children: ReactNode;
  currentStep: SetupStep;
}) {
  return (
    <main className="min-h-screen bg-void px-4 py-5 text-paper sm:px-6 sm:py-8">
      <div className="mx-auto max-w-[1200px]">
        <header className="mb-8 flex items-center justify-between gap-4">
          <BrandMark />
          <div className="text-right">
            <p className="text-xs font-medium uppercase tracking-[0.12em] text-fog">
              First-run setup
            </p>
            <p className="mt-1 text-xs text-fog">
              {setupSteps.find((item) => item.id === currentStep)?.label ??
                "Setup"}
            </p>
          </div>
        </header>
        {children}
        <footer className="mt-8 flex flex-col gap-2 border-t border-graphite pt-4 text-xs text-fog sm:flex-row sm:items-center sm:justify-between">
          <span>Stealth Console · setup state stays on the server</span>
          <Link
            href="/login"
            className="text-mist transition-colors duration-150 hover:text-acid-lime"
          >
            Already have access? Sign in
          </Link>
        </footer>
      </div>
    </main>
  );
}
