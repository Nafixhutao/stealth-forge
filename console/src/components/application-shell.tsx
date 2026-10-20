"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { Sidebar } from "@/features/navigation/sidebar";
import { PanelToggleIcon } from "@/features/navigation/sidebar-shared";

type ApplicationShellValue = {
  openSidebar: () => void;
};

const ApplicationContext = createContext<ApplicationShellValue | null>(null);

/** Pages inside the shell use this to open the mobile navigation sheet. */
export function useApplicationShell() {
  return useContext(ApplicationContext);
}

/**
 * Shared chrome for every route: responsive sidebar plus the content area.
 * Page content is passed as children so each page keeps its own client
 * boundary instead of the whole tree being one.
 */
export function ApplicationShell({
  children,
  desktopSidebar = true,
  hasTopBar = false,
  desktopHasTopBar = hasTopBar,
  userConsole = false,
}: {
  children: ReactNode;
  /** Pages with their own chrome can opt out of the desktop rail. */
  desktopSidebar?: boolean;
  /** Shifts the mobile drawer below the 59px top bar those pages render. */
  hasTopBar?: boolean;
  /** Whether the desktop rail clears the same bar. Defaults to `hasTopBar`;
   * a bar that is `lg:hidden` passes false so the rail keeps full height. */
  desktopHasTopBar?: boolean;
  /** Marks the shell as the user console, opting the surface into that
   * console's own typography and measured palette (see `admin.css`). */
  userConsole?: boolean;
}) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  // Stable identity: consumers (pages with heavy tables) must not re-render
  // just because the shell re-rendered.
  const openSidebar = useCallback(() => setSidebarOpen(true), []);
  const closeSidebar = useCallback(() => setSidebarOpen(false), []);
  const shellValue = useMemo(() => ({ openSidebar }), [openSidebar]);

  return (
    <ApplicationContext.Provider value={shellValue}>
      <div
        data-user
        data-console={userConsole ? "user" : undefined}
        className="min-h-dvh w-full max-w-full overflow-x-hidden bg-[var(--projects-bg)]"
      >
        {/* Mobile-only nav opener, pinned top-left like the CodeRabbit reference.
            Pages that render their own top bar provide the opener inside it
            instead, so this stays hidden there. Sits under the sheet overlay
            (z-50) so the backdrop covers it while open. */}
        {!hasTopBar && (
          <button
            type="button"
            onClick={openSidebar}
            aria-label="Open sidebar"
            aria-haspopup="dialog"
            className="fixed left-4 top-3 z-40 inline-flex size-9 items-center justify-center rounded-lg border border-[var(--projects-border)] bg-[var(--projects-control)] text-[var(--projects-text)] shadow-[0_1px_3px_rgba(0,0,0,0.35)] transition-colors hover:border-[var(--projects-border-hover)] lg:hidden"
          >
            <PanelToggleIcon className="size-[18px]" />
          </button>
        )}
        <div className="min-h-dvh lg:flex">
          <Sidebar
            open={sidebarOpen}
            onClose={closeSidebar}
            hasTopBar={hasTopBar}
            desktopHasTopBar={desktopHasTopBar}
            desktop={desktopSidebar}
          />
          <main className="relative min-h-dvh min-w-0 w-full max-w-full overflow-x-hidden flex-1">
            {children}
          </main>
        </div>
      </div>
    </ApplicationContext.Provider>
  );
}
