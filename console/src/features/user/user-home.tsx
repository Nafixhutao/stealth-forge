"use client";

import Link from "next/link";
import {
  Building2,
  ChevronDown,
  FolderKanban,
  LayoutGrid,
  List,
  Plus,
  Search,
} from "lucide-react";
import { useMemo, useState } from "react";
import { useCurrentAccount } from "@/api/queries/account";
import {
  useOrganizationPlan,
  useOrganizationProjects,
  useOrganizations,
} from "@/api/queries/organizations";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { PageBody } from "@/components/panel";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { CreateProjectDialog } from "@/features/user/create-project-dialog";
import {
  PRIMARY_ACTION,
  SECONDARY_ACTION,
} from "@/features/user/action-styles";
import {
  ProjectCard,
  ProjectCardsSkeleton,
  ProjectListSkeleton,
  ProjectRow,
} from "@/features/user/project-card";
import { UserTopBar } from "@/features/user/user-top-bar";
import { cn } from "@/lib/utils";

/**
 * User console home: an organization's projects.
 *
 * The layout follows the mobile reference top to bottom: the top bar (brand,
 * search, account), the organization row with its plan, a section tab row, a
 * one-line toolbar, the project cards, and a closing footer. Every value is
 * read from the API.
 *
 * Design reasons (R-31):
 * - Composition: one organization context, then its projects. The page is built
 *   around the decision "which project am I working in", not a summary dashboard.
 * - Box: the card grid is the default presentation, matching the Appwrite
 *   reference; the projects table box stays one toggle away. The ⋯ menu on
 *   both presentations is real — it copies the project id for everyone and
 *   opens the admin console for instance admins — and the card's Requests row
 *   is kept, reading N/A instead of a fabricated zero because request metering
 *   is not wired into this surface yet (R-17).
 * - Tabs: only Projects exists today. The rest stay inert and muted, with the
 *   reason on `title`, so a tab pointing at a page that does not exist is never
 *   a working control (R-24, R-26).
 * - Accent: the one accent is the primary Create project action, the org-row
 *   quick action, and the card usage line. The active tab underline stays
 *   neutral, and the selected state carries no colour, so the accent keeps
 *   its meaning.
 * - Colour: `--projects-*` tokens for surfaces and `--user-accent*` for the
 *   product accent, not fixed hex, because this area ships a light theme.
 * - Motion: hover, focus, and open/close only, matching the MOTION 1 dial.
 */

const FOCUS =
  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--user-accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--projects-bg)]";

const PLAN_LABEL: Record<string, string> = {
  free: "Free",
  pro: "Pro",
  enterprise: "Enterprise",
};

/**
 * Real usage against the plan limit, as one sentence. It rides on the plan
 * chip's tooltip so the organization row keeps the reference's compact shape
 * without dropping the number.
 */
function usageText(used: number, limit: number): string {
  if (limit < 0)
    return `${used} ${used === 1 ? "project" : "projects"}, unlimited`;
  return `${used} of ${limit} ${limit === 1 ? "project" : "projects"} used`;
}

/** Initials mark. The Console ships no avatar asset, so none is invented. */
function OrgAvatar({ label }: { label: string }) {
  return (
    <span
      aria-hidden="true"
      className="flex !size-6 shrink-0 items-center justify-center rounded-full bg-[var(--projects-chip)] text-[10px] font-semibold text-[var(--projects-text)] sm:size-10 sm:rounded-[10px] sm:text-[13px]"
    >
      {label.slice(0, 2).toUpperCase()}
    </span>
  );
}

/** Real plan badge, read from the plan endpoint. A filled chip, not a border. */
function PlanBadge({ planKey, title }: { planKey: string; title?: string }) {
  return (
    <span
      title={title}
      className="inline-flex !h-[15px] shrink-0 items-center rounded-md bg-[var(--projects-chip)] !px-1.5 leading-[15px] text-[9.5px]! font-medium text-[var(--projects-muted)] sm:h-auto sm:px-[7px] sm:py-[2px] sm:leading-4 sm:text-[11.5px]!"
    >
      {PLAN_LABEL[planKey] ?? planKey}
    </span>
  );
}

/** Closing footer: the brand mark and the operator-only console link. */
function UserFooter({
  canOpenAdmin,
  role,
}: {
  canOpenAdmin: boolean;
  role: string | null;
}) {
  return (
    <footer className="border-t border-[var(--projects-divider)]">
      <div className="mx-auto flex h-12 w-full max-w-[1440px] items-center gap-6 px-4 sm:h-14 sm:px-6 lg:px-7">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src="/stealth-cat.png"
          alt=""
          className="size-5 shrink-0 object-contain opacity-80"
        />
        {canOpenAdmin ? (
          <Link
            href="/admin"
            aria-label={`${
              role === "instance_owner" ? "Instance owner" : "Instance admin"
            } — open the admin console`}
            className={cn(
              "rounded-md text-[13px] leading-5 text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]",
              FOCUS,
            )}
          >
            Admin
          </Link>
        ) : null}
        <Link
          href="/admin/status"
          className={cn(
            "rounded-md text-[13px] leading-5 text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)]",
            FOCUS,
          )}
        >
          Status
        </Link>
        <p className="m-0 ml-auto text-[13px] leading-5 text-[var(--projects-muted)]">
          © {new Date().getFullYear()} Stealth
        </p>
      </div>
    </footer>
  );
}

export function UserHome() {
  const organizations = useOrganizations();
  const account = useCurrentAccount();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  // The card grid is the default (the Appwrite reference's presentation); the
  // compact table box stays one click away.
  const [view, setView] = useState<"grid" | "list">("grid");
  const [search, setSearch] = useState("");
  const [createOpen, setCreateOpen] = useState(false);

  const items = organizations.data?.organizations ?? [];
  // An explicit choice wins; otherwise the first organization is the default so
  // the page has something to show on first paint.
  const selected =
    items.find((organization) => organization.id === selectedId) ??
    items[0] ??
    null;

  const plan = useOrganizationPlan(selected?.id);
  const projects = useOrganizationProjects(selected?.id);

  const projectItems = useMemo(
    () => projects.data?.projects ?? [],
    [projects.data],
  );
  const visible = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return projectItems;
    return projectItems.filter((project) =>
      project.name.toLowerCase().includes(query),
    );
  }, [projectItems, search]);

  const role = account.data?.account.instance_role ?? null;
  const canOpenAdmin = role === "instance_owner" || role === "instance_admin";

  const orgPlan = plan.data?.plan;
  const orgsLoading = organizations.isPending;
  // The toolbar is a set of controls over the project list, so it is withheld
  // when there is no list to act on: zero projects reads as a first-run
  // surface, and the empty state owns the create action there.
  const showToolbar = projects.isPending || projectItems.length > 0;

  return (
    <div className="flex min-h-dvh w-full max-w-full flex-col overflow-x-hidden">
      <UserTopBar canOpenAdmin={canOpenAdmin} />

      <div className="flex-1 w-full max-w-full overflow-x-hidden">
        <PageBody>
          {/* Organization context: who the projects below belong to, and its
              plan. The plan chip's tooltip carries the real usage numbers. */}
          <header className="flex items-center gap-2 px-4 sm:gap-3 sm:px-0">
            {orgsLoading ? (
              <span className="!size-[26px] shrink-0 animate-pulse rounded-full bg-[var(--projects-chip)] motion-reduce:animate-none sm:size-10 sm:rounded-[10px]" />
            ) : (
              <OrgAvatar label={selected?.name ?? "?"} />
            )}
            <div className="flex min-w-0 items-center gap-2">
              <h1 className="m-0 truncate !text-[13px] font-semibold leading-6 tracking-[-0.02em] text-[var(--projects-text)] sm:text-[18px]">
                {orgsLoading
                  ? "Loading organizations"
                  : (selected?.name ?? "No organization")}
              </h1>
              {orgPlan ? (
                <PlanBadge
                  planKey={orgPlan.plan_key}
                  title={usageText(
                    orgPlan.usage.projects,
                    orgPlan.limits.projects,
                  )}
                />
              ) : null}
              {selected ? (
                // Always offered: the reference keeps the switcher visible even
                // with one organization, where the menu names it as current.
                <DropdownMenu>
                  <DropdownMenuTrigger
                    title="Switch organization"
                    className={cn(
                      "-ml-1.5 inline-flex min-h-10 shrink-0 items-center rounded-md px-1.5 text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)] sm:ml-0 sm:min-h-9",
                      FOCUS,
                    )}
                    aria-label="Switch organization"
                  >
                    <ChevronDown
                      size={16}
                      strokeWidth={2}
                      className="!size-3.5 sm:!size-4"
                      aria-hidden="true"
                    />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="start" data-user-surface="">
                    <DropdownMenuLabel>Switch organization</DropdownMenuLabel>
                    {items.map((organization) => (
                      <DropdownMenuItem
                        key={organization.id}
                        onSelect={() => {
                          setSelectedId(organization.id);
                          setSearch("");
                        }}
                      >
                        <span className="truncate">{organization.name}</span>
                        {organization.id === selected.id ? (
                          <span className="ml-auto text-[11px] uppercase tracking-[0.08em] text-[var(--projects-muted)]">
                            Current
                          </span>
                        ) : null}
                      </DropdownMenuItem>
                    ))}
                  </DropdownMenuContent>
                </DropdownMenu>
              ) : null}
              <button
                type="button"
                disabled={!selected}
                onClick={() => setCreateOpen(true)}
                aria-label="Create project"
                title="Create project"
                className={cn(
                  "ml-[5px] sm:ml-[18px] inline-flex size-6 shrink-0 items-center justify-center text-[var(--projects-muted)] transition-colors hover:text-[var(--projects-text)] disabled:pointer-events-none disabled:opacity-40",
                  FOCUS,
                )}
              >
                <Plus size={16} strokeWidth={2} aria-hidden="true" />
              </button>
            </div>
          </header>

          {/* Section tabs. Only Projects exists, so the rest are inert: muted,
              with the reason on `title`, instead of pointing at a page that is
              not built. */}
          <nav
            aria-label="Organization sections"
            className="-mt-1 flex w-full min-w-0 items-center overflow-x-auto border-b border-[var(--projects-border)] px-4 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden sm:mt-0 sm:px-0"
          >
            <span
              aria-current="page"
              className="relative inline-flex h-[44px] shrink-0 items-center whitespace-nowrap px-3 !text-[13px] font-medium text-[var(--projects-text)] !border-b-2 !border-white sm:h-[52px]"
            >
              Projects
            </span>
            {["Marketplace", "Domains", "Settings"].map((label) => (
              <span
                key={label}
                aria-disabled="true"
                title="Coming soon"
                className="inline-flex h-[44px] shrink-0 items-center whitespace-nowrap px-3 !text-[13px] font-medium text-[var(--projects-muted)] !border-b-2 !border-transparent sm:h-[52px]"
              >
                {label}
              </span>
            ))}
          </nav>

          {organizations.isError ? (
            <ErrorState
              error={organizations.error}
              retry={() => void organizations.refetch()}
              title="Unable to load organizations"
            />
          ) : !orgsLoading && items.length === 0 ? (
            <EmptyState
              className="py-10 sm:py-14"
              icon={Building2}
              title="You are not in an organization yet"
              description="An organization groups projects, members, and billing. Accept an invitation from an existing organization, or ask the instance owner to add you to one."
            />
          ) : (
            <>
              {/* Toolbar: filter, view mode, and the one primary action on a
                  single line at every width — matching the Appwrite reference
                  at exact sizes: the field takes the room left over from the
                  62px view toggle, the 12px gaps, and the primary action. Every
                  control shares one 36px height. */}
              {showToolbar ? (
                <div className="flex w-full items-center gap-3 px-4 sm:px-0">
                  <div className="relative min-w-0 flex-1 sm:max-w-[380px]">
                    <Search
                      size={16}
                      strokeWidth={1.8}
                      aria-hidden="true"
                      className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--projects-muted)]"
                    />
                    <Input
                      value={search}
                      onChange={(event) => setSearch(event.target.value)}
                      placeholder="Search projects"
                      aria-label="Search projects"
                      // The shared Input is dark-only (`bg-carbon`, lavender focus).
                      // This surface ships a light theme and its own accent, so the
                      // field is re-themed with the `--projects-*` and `--user-*`
                      // tokens; twMerge drops the dark utilities.
                      className="!h-9 !min-h-0 rounded-lg border-[var(--projects-border)] bg-[var(--projects-control)] pl-10 pr-2.5 !text-[13px] text-[var(--projects-text)] placeholder:text-[var(--projects-muted)] focus-visible:border-[var(--user-accent)]/70 focus-visible:ring-[var(--user-accent)]/15 sm:!text-[15px]"
                    />
                  </div>

                  <div
                    role="group"
                    aria-label="Project view"
                    className="flex !h-[34px] w-[62px] shrink-0 items-center rounded-lg bg-[var(--projects-control)] p-[3px] sm:!h-9 sm:ml-auto sm:w-auto"
                  >
                    {[
                      {
                        mode: "list" as const,
                        icon: List,
                        label: "List view",
                      },
                      {
                        mode: "grid" as const,
                        icon: LayoutGrid,
                        label: "Grid view",
                      },
                    ].map(({ mode, icon: Icon, label }) => (
                      <button
                        key={mode}
                        type="button"
                        aria-pressed={view === mode}
                        aria-label={label}
                        onClick={() => setView(mode)}
                        className={cn(
                          "inline-flex size-7 items-center justify-center rounded-lg text-[var(--projects-muted)] transition-colors",
                          // The reference marks the selected segment by cutting
                          // it back to the page colour rather than lifting it.
                          view === mode
                            ? "bg-[var(--projects-bg)]"
                            : "hover:text-[var(--projects-text)]",
                          FOCUS,
                        )}
                      >
                        <Icon
                          className="size-4 shrink-0"
                          strokeWidth={1.8}
                          aria-hidden="true"
                        />
                      </button>
                    ))}
                  </div>

                  <Button
                    type="button"
                    disabled={!selected}
                    onClick={() => setCreateOpen(true)}
                    className={PRIMARY_ACTION}
                  >
                    <Plus
                      className="size-4 shrink-0"
                      strokeWidth={2.2}
                      aria-hidden="true"
                    />
                    Create project
                  </Button>
                </div>
              ) : null}

              {selected ? (
                <CreateProjectDialog
                  organizationId={selected.id}
                  organizationName={selected.name}
                  open={createOpen}
                  onOpenChange={setCreateOpen}
                />
              ) : null}

              {/* The list itself. */}
              {projects.isPending ? (
                view === "grid" ? (
                  <ProjectCardsSkeleton count={3} />
                ) : (
                  <ProjectListSkeleton count={3} />
                )
              ) : projects.isError ? (
                <ErrorState
                  error={projects.error}
                  retry={() => void projects.refetch()}
                  title="Unable to load projects"
                />
              ) : projectItems.length === 0 ? (
                <EmptyState
                  className="py-10 sm:py-14"
                  icon={FolderKanban}
                  title="No projects in this organization yet"
                  description="Create the first project to get a place for its databases, storage, functions, and deployments."
                  action={
                    <Button
                      type="button"
                      onClick={() => setCreateOpen(true)}
                      className={PRIMARY_ACTION}
                    >
                      Create project
                    </Button>
                  }
                />
              ) : visible.length === 0 ? (
                <EmptyState
                  className="py-10 sm:py-14"
                  icon={Search}
                  title="No projects match this search"
                  description={`Nothing in ${selected?.name ?? "this organization"} matches ${search.trim()}. Clear the search to see every project.`}
                  action={
                    <Button
                      type="button"
                      variant="secondary"
                      onClick={() => setSearch("")}
                      className={SECONDARY_ACTION}
                    >
                      Clear search
                    </Button>
                  }
                />
              ) : view === "grid" ? (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
                  {visible.map((project) => (
                    <ProjectCard
                      key={project.id}
                      project={project}
                      canOpenAdmin={canOpenAdmin}
                      onCreateProject={() => setCreateOpen(true)}
                    />
                  ))}
                </div>
              ) : (
                // The default presentation: the reference's compact table box.
                // The identity cell carries the created date as a sub-line, the
                // second column the project id, and the closing row reports how
                // many of the organization's projects are on screen.
                <div className="mx-4 overflow-hidden rounded-xl border border-[var(--projects-border)] bg-[var(--projects-card-bg)] sm:mx-0">
                  <table className="w-full table-fixed border-collapse text-left">
                    <thead>
                      <tr className="border-b border-[var(--projects-divider)]">
                        <th
                          scope="col"
                          className="px-4 py-2.5 text-[12px] font-normal text-[var(--projects-muted)] sm:px-5 sm:py-3 sm:text-[12.5px]"
                        >
                          Project name
                        </th>
                        <th
                          scope="col"
                          className="w-[30%] px-4 py-2.5 text-[12px] font-normal text-[var(--projects-muted)] sm:px-5 sm:py-3 sm:text-[12.5px]"
                        >
                          Project ID
                        </th>
                        <th scope="col" className="w-12 px-2">
                          <span className="sr-only">Actions</span>
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      {visible.map((project) => (
                        <ProjectRow
                          key={project.id}
                          project={project}
                          canOpenAdmin={canOpenAdmin}
                        />
                      ))}
                      <tr>
                        <td
                          colSpan={3}
                          className="px-4 py-2.5 text-[12.5px] leading-5 text-[var(--projects-muted)] sm:px-5 sm:py-3"
                        >
                          {`Showing ${visible.length} of ${projectItems.length} ${
                            projectItems.length === 1 ? "project" : "projects"
                          }`}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              )}
            </>
          )}
        </PageBody>
      </div>

      <UserFooter canOpenAdmin={canOpenAdmin} role={role} />
    </div>
  );
}
