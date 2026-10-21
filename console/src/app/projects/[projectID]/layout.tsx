import "@fontsource-variable/inter";

import { ApplicationShell } from "@/components/application-shell";
import { ProjectShell } from "@/features/project/project-shell";

/**
 * The project surface's frame.
 *
 * Declared as a layout rather than repeated per page so the project read, the
 * identity header, and the section tabs mount once and survive navigation
 * between sections. `userConsole` opts the shell into the user console's
 * palette, and Inter is declared here for the same reason the organization home
 * declares it: the shell's other routes keep the admin console's Geist.
 */
export default async function ProjectLayout({
  params,
  children,
}: {
  params: Promise<{ projectID: string }>;
  children: React.ReactNode;
}) {
  const { projectID } = await params;

  return (
    <ApplicationShell hasTopBar desktopHasTopBar={false} userConsole>
      <ProjectShell projectId={projectID}>{children}</ProjectShell>
    </ApplicationShell>
  );
}
