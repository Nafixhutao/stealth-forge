"use client";

import { useMemo, type ReactNode } from "react";
import { useCurrentAccount } from "@/api/queries/account";
import { useConsoleRouteContext } from "@/components/navigation/console-route-context";
import { NavIcon } from "./sidebar-shared";
import { userNav, type UserNavLeaf } from "./user-nav";
import type { NavItem } from "./types";

/** A navigation row that carries a real destination. */
export type NavLink = NavItem & { href: string };

/** An expandable navigation group (the redesign's "Review" pattern). */
export type NavGroupData = {
  label: string;
  icon: ReactNode;
  items: NavLink[];
};

export type UserNavData = {
  primary: NavLink[];
  groups: NavGroupData[];
  admin?: NavLink;
};

function toLink(leaf: UserNavLeaf): NavLink {
  return {
    icon: NavIcon(leaf.icon),
    label: leaf.label,
    href: leaf.href,
    badge: leaf.badge,
  };
}

/**
 * Builds the sidebar's rows from the console's real user routes and the signed
 * in account, in the exact shape the redesigned sidebar renders.
 */
export function useUserNav(): UserNavData {
  const { organizationId, projectId } = useConsoleRouteContext();
  const account = useCurrentAccount();
  const instanceAdmin =
    account.data?.account.instance_role === "instance_owner" ||
    account.data?.account.instance_role === "instance_admin";

  return useMemo(() => {
    const nav = userNav({ organizationId, projectId, instanceAdmin });
    return {
      primary: nav.primary.map(toLink),
      groups: nav.groups.map((group) => ({
        label: group.label,
        icon: group.items[0] ? NavIcon(group.items[0].icon) : null,
        items: group.items.map(toLink),
      })),
      admin: nav.admin ? toLink(nav.admin) : undefined,
    };
  }, [organizationId, projectId, instanceAdmin]);
}
