import type { ReactNode } from "react";

export interface SidebarProps {
  /** Controls the slide-in mobile drawer. Desktop is always visible. */
  open: boolean;
  onClose: () => void;
  /** Keeps the sidebar below the 48px application top bar when present. */
  hasTopBar?: boolean;
  /** When false the desktop rail is never rendered — pages that manage their
   * own chrome still get the mobile drawer without changing desktop layout. */
  desktop?: boolean;
}

export type BadgeKind = "Beta" | "New";

export type NavRowProps = {
  icon: ReactNode;
  label: string;
  badge?: BadgeKind;
  expandable?: boolean;
  labelClassName?: string;
  /** Renders the shared layoutId pill behind the row when selected. */
  isActive?: boolean;
  /** Shared layoutId — every row in a sidebar passes the same one so the
   * active pill morphs between rows instead of remounting. */
  layoutId?: string;
  onSelect?: () => void;
  /** Desktop rail mode: labels fade out and the row keeps its icon only. */
  collapsed?: boolean;
};

export type NavItem = Pick<NavRowProps, "icon" | "label" | "badge" | "expandable" | "labelClassName">;
