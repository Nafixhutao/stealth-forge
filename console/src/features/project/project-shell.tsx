"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ChevronLeft, Copy } from "lucide-react";
import { toast } from "sonner";
import { ApiError } from "@/api/client";
import { useCurrentAccount } from "@/api/queries/account";
import {
  organizationItems,
  useOrganizations,
} from "@/api/queries/organizations";
import { useProject } from "@/api/queries/projects";
import { ErrorState } from "@/components/feedback/error-state";
import { PageBody } from "@/components/panel";
import { projectPath } from "@/features/project/paths";
import { ProjectMark } from "@/features/user/project-card";
import { UserFooter } from "@/features/user/user-footer";
import { UserTopBar } from "@/features/user/user-top-bar";
import { cn } from "@/lib/utils";

/**
 * The project surface's frame: the project's identity, its section tabs, and
 * the closing footer, wrapped around whichever section is mounted.
 *
 * The frame owns the project read, so every section below it can assume the
 * project resolved — a 404 (the project does not exist, or the account has no
 * membership in its organization) is answered once here instead of once per
 * section, and sections are not mounted at all in that case, which keeps a
 * missing project from firing a request per tab.
 *
 * Destructive project actions are deliberately absent: the API's project
 * response carries no `can_manage`, and every resource list reports its own, so
 * permissions are surfaced where they are known rather than guessed here.
 */

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--projects-bg)]";

/** Section tabs, in product order. `segment` is empty for the Overview. */
const SECTIONS = [
  { label: "Overview", segment: "" },
  { label: "Databases", segment: "databases" },
  { label: "Storage", segment: "storage" },
  { label: "Functions", segment: "functions" },
  { label: "Sites", segment: "sites" },
] as const;

/** The project's section tabs, in product order. */
function ProjectTabs({ projectId }: { projectId: string }) {
  const pathname = usePathname();

  return (
    <nav
      aria-label="Project sections"
      className="-mt-1 flex w-full min-w-0 items-center overflow-x-auto border-b border-[var(--projects-border)] px-4 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden sm:mt-0 sm:px-0"
    >
      {SECTIONS.map((section) => {
        const href = projectPath(projectId, section.segment);
        // A detail route (`/databases/<id>`) keeps its section tab selected.
        const active =
          section.segment === ""
            ? pathname === href
            : pathname === href || pathname.startsWith(`${href}/`);
        return (
          <Link
            key={section.label}
            href={href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "inline-flex h-[44px] shrink-0 items-center whitespace-nowrap px-3 !text-[13px] font-medium !border-b-2 sm:h-[52px]",
              active
                ? "text-[var(--projects-text)] !border-[var(--projects-text)]"
                : "text-[var(--projects-muted)] !border-transparent transition-colors hover:text-[var(--projects-text)]",
              FOCUS,
            )}
          >
            {section.label}
          </Link>
        );
      })}
    </nav>
  );
}

/**
 * The project id, copyable. It is the value the CLI and the SDKs need, so it
 * sits in the header rather than behind a menu.
 */
function ProjectIdChip({ projectId }: { projectId: string }) {
  const copy = () => {
    void navigator.clipboard?.writeText(projectId).catch(() => undefined);
    toast.success("Project ID copied");
  };

  return (
    <button
      type="button"
      onClick={copy}
      title={`Copy project ID ${projectId}`}
      aria-label="Copy project ID"
      className={cn(
        "ml-auto inline-flex min-h-9 shrink-0 items-center gap-1.5 rounded-lg px-2 text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]",
        FOCUS,
      )}
    >
      <Copy size={14} strokeWidth={1.8} aria-hidden="true" />
      <span className="admin-mono hidden text-[11.5px] sm:inline">
        {projectId.slice(0, 8)}
      </span>
    </button>
  );
}

function ProjectHeader({ projectId }: { projectId: string }) {
  const organizations = useOrganizations();
  const project = useProject(projectId);
  const organizationId = project.data?.project.organization_id;
  const organization = organizationItems(organizations.data).find(
    (item) => item.id === organizationId,
  );
  const heading = project.data?.project.name;

  return (
    <header className="flex items-center gap-2 px-4 sm:gap-3 sm:px-0">
      <ProjectMark />
      <div className="flex min-w-0 flex-col">
        <Link
          href="/user"
          className={cn(
            "inline-flex items-center gap-0.5 truncate rounded text-[12px] leading-5 text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]",
            FOCUS,
          )}
        >
          <ChevronLeft size={13} strokeWidth={2} aria-hidden="true" />
          {organization?.name ?? "All projects"}
        </Link>
        {heading ? (
          <h1
            title={heading}
            className="m-0 truncate !text-[15px] font-semibold leading-6 tracking-[-0.02em] text-[var(--projects-text)] sm:!text-[18px]"
          >
            {heading}
          </h1>
        ) : project.isError ? (
          // No name was ever read, and a loading placeholder would pulse for as
          // long as the error stands. The id is what the URL asked for, so that
          // is what the header keeps.
          <span
            title={projectId}
            className="admin-mono block truncate text-[13px] leading-6 text-[var(--projects-muted)]"
          >
            {projectId}
          </span>
        ) : (
          <span className="my-1 block h-4 w-40 max-w-full animate-pulse rounded bg-[var(--projects-chip)] motion-reduce:animate-none" />
        )}
      </div>
      {project.isError ? null : <ProjectIdChip projectId={projectId} />}
    </header>
  );
}

export function ProjectShell({
  projectId,
  children,
}: {
  projectId: string;
  children: React.ReactNode;
}) {
  const account = useCurrentAccount();
  const project = useProject(projectId);
  const role = account.data?.account.instance_role ?? null;
  const canOpenAdmin = role === "instance_owner" || role === "instance_admin";

  const missing =
    project.error instanceof ApiError && project.error.status === 404;

  return (
    <div className="flex min-h-dvh w-full max-w-full flex-col overflow-x-hidden">
      <UserTopBar canOpenAdmin={canOpenAdmin} />

      <div className="flex-1 w-full max-w-full overflow-x-hidden">
        <PageBody>
          <ProjectHeader projectId={projectId} />

          {project.isError ? (
            <ErrorState
              error={project.error}
              retry={missing ? undefined : () => void project.refetch()}
              title={
                missing ? "Project not found" : "Unable to load this project"
              }
            />
          ) : (
            <>
              <ProjectTabs projectId={projectId} />
              {children}
            </>
          )}
        </PageBody>
      </div>

      <UserFooter canOpenAdmin={canOpenAdmin} role={role} />
    </div>
  );
}
