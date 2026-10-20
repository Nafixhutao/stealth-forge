import type { ReactNode } from "react";

export interface SidebarProps {
  /** Controls the slide-in mobile drawer. Desktop is always visible. */
  open: boolean;
  onClose: () => void;
  /** Keeps the sidebar below the 59px application top bar when present. */
  hasTopBar?: boolean;
  /** Whether the desktop rail also clears a 59px top bar. Defaults to
   * `hasTopBar`; pass false for a bar that only exists below lg. */
  desktopHasTopBar?: boolean;
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
  /** Called on pointer hover or focus, before the row is activated. Rows that
   * load a heavier surface on demand use it to warm that chunk. */
  onIntent?: () => void;
  /** Desktop rail mode: labels fade out and the row keeps its icon only. */
  collapsed?: boolean;
};

export type NavItem = Pick<
  NavRowProps,
  "icon" | "label" | "badge" | "expandable" | "labelClassName"
>;
