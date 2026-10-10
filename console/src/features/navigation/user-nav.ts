import {
  sidebarNav,
  type NavItem,
  type NavSection,
} from "@/components/layout/sidebar-nav";
import type { BadgeKind } from "./types";

/** A clickable sidebar destination. */
export type UserNavLeaf = {
  label: string;
  href: string;
  icon: NavItem["icon"];
  exact?: boolean;
  badge?: BadgeKind;
};

/** An expandable sidebar group (the redesign's "Review" pattern). */
export type UserNavGroup = {
  label: string;
  items: UserNavLeaf[];
};

export type UserNav = {
  /** Always-visible rows, rendered flat like the redesigned primary nav. */
  primary: UserNavLeaf[];
  /** Collapsible groups, one per project section. */
  groups: UserNavGroup[];
  /** Instance-admin shortcut, kept last. */
  admin?: UserNavLeaf;
};

type UserNavParams = {
  organizationId?: string;
  projectId?: string;
  instanceAdmin: boolean;
};

function toLeaf(item: NavItem): UserNavLeaf {
  return {
    label: item.label,
    href: item.href,
    icon: item.icon,
    exact: item.exact,
  };
}

/**
 * Adapts the console's route-aware sections into the redesigned sidebar shape:
 * the workspace rows plus the project's primary rows stay flat, while the
 * remaining project sections become collapsible groups. Every destination is
 * exactly the one the console already exposes — nothing is invented.
 */
export function userNav(params: UserNavParams): UserNav {
  const nav = sidebarNav(params);
  const projectSections: NavSection[] = nav.project ?? [];

  // The first project section (Overview / Services / Deployments) reads as the
  // project's primary navigation, so it joins the flat rows; the rest become
  // expandable groups to keep the panel calm.
  const [projectPrimary, ...projectGroups] = projectSections;

  return {
    primary: [
      ...nav.organization.items.map(toLeaf),
      ...(projectPrimary?.items.map(toLeaf) ?? []),
    ],
    groups: projectGroups.map((section) => ({
      label: section.label,
      items: section.items.map(toLeaf),
    })),
    admin: nav.admin ? toLeaf(nav.admin.items[0]) : undefined,
  };
}
