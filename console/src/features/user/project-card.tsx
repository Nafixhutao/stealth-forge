"use client";

import Link from "next/link";
import { FolderKanban, MoreHorizontal, Plus } from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { Mono } from "@/components/panel";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { projectPath } from "@/features/project/paths";
import { formatCount } from "@/lib/format";
import { cn } from "@/lib/utils";

/**
 * Project presentation for the organization home: the table box (the default
 * view), the card grid, and their loading placeholders. Split out of
 * `user-home.tsx` so the page file keeps orchestration while the project-tile
 * contract lives here.
 *
 * The card is the default presentation and follows the Appwrite reference: an
 * identity block (name and created date), the Requests row, the accent chart
 * area, and a closing row with the dashed quick-create action and the ⋯ menu.
 * The ⋯ menu is real on both presentations: it copies the project id for
 * everyone and opens the admin console for instance admins. The table box
 * follows the compact reference: an identity cell (icon, name, and created
 * date), the project id, and the row actions. The card grid stays one compact
 * card per row on phones, the roomier tile from `sm` up. Every card keeps its
 * Requests row (metering is not wired into this surface yet, so the value
 * reads N/A instead of a fabricated zero) and the accent usage line.
 */

const MONTHS = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
];

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--projects-bg)]";

function formatUtcDate(value: string): string | null {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return `${date.getUTCDate()} ${MONTHS[date.getUTCMonth()]} ${date.getUTCFullYear()}`;
}

/**
 * The project's identity mark: a tinted square with the project glyph. Shared
 * by the box row and the project header, so a project looks the same wherever
 * it is named.
 */
export function ProjectMark({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex size-8 shrink-0 items-center justify-center rounded-lg bg-[color-mix(in_srgb,var(--user-accent)_12%,transparent)] sm:size-9",
        className,
      )}
    >
      <FolderKanban
        size={16}
        strokeWidth={1.8}
        className="text-[var(--user-accent)]"
      />
    </span>
  );
}

/**
 * The actions menu shared by the table row and the card. Callers pass their
 * own trigger skin so each presentation keeps its density.
 */
function ProjectActionsMenu({
  projectId,
  canOpenAdmin,
  triggerClassName,
  iconSize,
}: {
  projectId: string;
  canOpenAdmin: boolean;
  triggerClassName: string;
  iconSize: number;
}) {
  const router = useRouter();

  const copyId = () => {
    // Mock-data friendly: the clipboard may be unavailable; the toast still
    // confirms the intent.
    void navigator.clipboard?.writeText(projectId).catch(() => undefined);
    toast.success("Project ID copied");
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="Project actions"
        title="Project actions"
        className={cn(triggerClassName, FOCUS)}
      >
        <MoreHorizontal size={iconSize} strokeWidth={1.8} aria-hidden="true" />
      </DropdownMenuTrigger>
      {/* Portaled to the body, so it opts into the console's light palette and
          accent with `data-user-surface`. */}
      <DropdownMenuContent align="end" data-user-surface="">
        <DropdownMenuItem onSelect={() => router.push(projectPath(projectId))}>
          Open project
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={copyId}>Copy project ID</DropdownMenuItem>
        {canOpenAdmin ? (
          <DropdownMenuItem onSelect={() => router.push("/admin")}>
            Open admin console
          </DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** One project, rendered as a card: identity, metric, usage line, actions. */
export function ProjectCard({
  project,
  requests,
  canOpenAdmin,
  onCreateProject,
}: {
  project: { id: string; name: string; created_at: string };
  /**
   * Requests in the last 30 days, from the project's usage snapshot.
   * `undefined` while that read is in flight or if it failed; the card says so
   * rather than printing a zero it did not measure.
   */
  requests: number | undefined;
  canOpenAdmin: boolean;
  onCreateProject: () => void;
}) {
  const created = formatUtcDate(project.created_at);
  const href = projectPath(project.id);

  return (
    <article className="relative mx-4 flex flex-col overflow-hidden rounded-[13px] border border-[var(--projects-border)] bg-[var(--projects-card-bg)] divide-y divide-[var(--projects-divider)] transition-colors hover:border-[var(--projects-border-hover)] sm:mx-0">
      <div className="flex min-h-[65px] flex-col justify-center py-[12.5px] !px-4">
        <h3
          title={project.name}
          className="m-0 truncate !text-[14px] font-semibold leading-5 tracking-[-0.01em] text-[var(--projects-text)]"
        >
          {/* The link's `after` overlay makes the whole card a target for the
              pointer; the action row below paints above it. */}
          <Link
            href={href}
            className={cn(
              "block truncate no-underline after:absolute after:inset-0 after:content-['']",
              FOCUS,
            )}
          >
            {project.name}
          </Link>
        </h3>
        <p className="m-0 truncate !text-[12px] leading-5 text-[var(--projects-muted)]">
          {created ? `Created ${created}` : "Creation date unavailable"}
        </p>
      </div>

      <div className="flex !h-[92px] box-border flex-col justify-between !px-4 pt-3 pb-2">
        <div className="flex items-baseline gap-2">
          <span className="text-[12px] leading-5 text-[var(--projects-muted)]">
            Requests
          </span>
          {requests === undefined ? (
            // A placeholder keeps the row's height while the snapshot loads, and
            // is never mistaken for a count.
            <span
              aria-hidden="true"
              className="h-3 w-8 animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none"
            />
          ) : (
            <span className="text-[12px] font-medium leading-5 text-[var(--projects-muted)]">
              {formatCount(requests)}
            </span>
          )}
          {requests === undefined ? (
            <span className="sr-only">Requests in the last 30 days loading</span>
          ) : null}
        </div>
        {/* The accent line sits at the bottom, exactly against the divider with pb-2 */}
        <div className="h-px bg-[var(--user-accent)]/70" aria-hidden="true" />
      </div>

      <div className="relative flex items-center justify-between gap-2 !px-4 py-[13px]">
        <button
          type="button"
          onClick={onCreateProject}
          aria-label="Create project"
          title="Create project"
          className={cn(
            "inline-flex !size-7 shrink-0 items-center justify-center rounded-lg border border-dashed border-[var(--projects-border-hover)] text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]",
            FOCUS,
          )}
        >
          <Plus size={15} strokeWidth={2} aria-hidden="true" />
        </button>

        <ProjectActionsMenu
          projectId={project.id}
          canOpenAdmin={canOpenAdmin}
          iconSize={16}
          triggerClassName="inline-flex size-7 shrink-0 items-center justify-center rounded-lg text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]"
        />
      </div>
    </article>
  );
}

/**
 * One project, rendered as a box row: the identity cell (mark, name, created
 * date), the project id, and the row actions. Name and id truncate on narrow
 * widths; both keep a tooltip with the full value.
 */
export function ProjectRow({
  project,
  canOpenAdmin,
}: {
  project: { id: string; name: string; created_at: string };
  canOpenAdmin: boolean;
}) {
  const created = formatUtcDate(project.created_at);
  return (
    <tr className="border-b border-[var(--projects-divider)] transition-colors hover:bg-[var(--projects-control)]">
      <td className="px-4 py-2.5 sm:px-5 sm:py-3">
        <div className="flex min-w-0 items-center gap-3">
          <ProjectMark />
          <span className="min-w-0">
            <Link
              href={projectPath(project.id)}
              title={project.name}
              className={cn(
                "block truncate text-[14px] font-semibold leading-5 text-[var(--projects-text)] no-underline sm:text-[15px]",
                FOCUS,
              )}
            >
              {project.name}
            </Link>
            <span className="block truncate text-[12.5px] leading-5 text-[var(--projects-muted)] sm:text-[13px]">
              {created ? `Created ${created}` : "Creation date unavailable"}
            </span>
          </span>
        </div>
      </td>
      <td className="px-4 py-2.5 sm:px-5 sm:py-3">
        <span className="block truncate" title={project.id}>
          <Mono className="text-[12px] text-[var(--projects-muted)]">
            {project.id}
          </Mono>
        </span>
      </td>
      <td className="w-12 px-2 py-2 text-right">
        <ProjectActionsMenu
          projectId={project.id}
          canOpenAdmin={canOpenAdmin}
          iconSize={16}
          triggerClassName="inline-flex size-8 items-center justify-center rounded-lg text-[var(--projects-muted)] transition-colors hover:bg-[var(--projects-control)] hover:text-[var(--projects-text)]"
        />
      </td>
    </tr>
  );
}

/** Loading placeholder with the box's footprint, so the table does not jump. */
export function ProjectListSkeleton({ count }: { count: number }) {
  return (
    <div className="mx-4 overflow-hidden rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] sm:mx-0">
      {Array.from({ length: count }).map((_, index) => (
        <div
          key={index}
          className="flex items-center gap-3 border-b border-[var(--projects-divider)] px-4 py-2.5 last:border-b-0 sm:px-5 sm:py-3"
        >
          <span className="size-8 shrink-0 animate-pulse rounded-lg bg-[var(--projects-control)] motion-reduce:animate-none sm:size-9" />
          <span className="min-w-0 flex-1 space-y-2">
            <span className="block h-3.5 w-40 max-w-full animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none" />
            <span className="block h-3 w-24 max-w-full animate-pulse rounded bg-[var(--projects-control)] motion-reduce:animate-none" />
          </span>
        </div>
      ))}
    </div>
  );
}

/** Loading placeholder with the card's footprint, so the grid does not jump. */
export function ProjectCardsSkeleton({ count }: { count: number }) {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {Array.from({ length: count }).map((_, index) => (
        <div
          key={index}
          className="mx-4 h-[215px] animate-pulse rounded-[13px] border border-[var(--projects-border)] bg-[var(--projects-card-bg)] motion-reduce:animate-none sm:mx-0"
        />
      ))}
    </div>
  );
}
