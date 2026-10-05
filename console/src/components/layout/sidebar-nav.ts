import {
  Activity,
  AppWindow,
  Bot,
  Boxes,
  Cable,
  CloudCog,
  Database,
  FolderKanban,
  Gauge,
  Globe2,
  KeyRound,
  Layers3,
  MessageSquare,
  Settings2,
  ShieldCheck,
  Users,
  Webhook,
  Zap,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import {
  organizationPath,
  organizationProjectsPath,
  organizationsPath,
  projectPath,
} from "@/lib/console-routes";

export type NavItem = {
  label: string;
  href: string;
  icon: LucideIcon;
  exact?: boolean;
};

export type NavSection = {
  label: string;
  items: NavItem[];
};

type SidebarNavParams = {
  organizationId?: string;
  projectId?: string;
  instanceAdmin: boolean;
};

export type SidebarNav = {
  organization: NavSection;
  /** Project-scoped sections; undefined when no project is selected. */
  project?: NavSection[];
  /** Instance-admin section; undefined for non-admin accounts. */
  admin?: NavSection;
};

function adminSection(): NavSection {
  return {
    label: "Admin",
    items: [
      { label: "Admin console", href: "/admin", icon: Gauge, exact: true },
    ],
  };
}

/**
 * Builds the sidebar's navigation from the active route context. The layout
 * owns rendering; this module owns which destinations exist and how the group
 * labels change with the selected organization and project.
 */
export function sidebarNav({
  organizationId,
  projectId,
  instanceAdmin,
}: SidebarNavParams): SidebarNav {
  const projectContext =
    organizationId && projectId ? { organizationId, projectId } : undefined;

  const organization: NavSection = {
    label: projectContext ? "Workspace" : "Organization",
    items: organizationId
      ? [
          {
            label: "Projects",
            href: organizationProjectsPath(organizationId),
            icon: FolderKanban,
            exact: true,
          },
          {
            label: "Members",
            href: organizationPath(organizationId, "members"),
            icon: Users,
          },
          {
            label: "Plan & limits",
            href: organizationPath(organizationId, "plan"),
            icon: Gauge,
          },
          {
            label: "Incidents",
            href: organizationPath(organizationId, "incidents"),
            icon: Activity,
          },
          {
            label: "Audit",
            href: organizationPath(organizationId, "audit"),
            icon: ShieldCheck,
          },
        ]
      : [
          {
            label: "Organizations",
            href: organizationsPath(),
            icon: FolderKanban,
            exact: true,
          },
        ],
  };

  const nav: SidebarNav = { organization };
  if (instanceAdmin) nav.admin = adminSection();
  if (!projectContext) return nav;

  const href = (segment: string) =>
    projectPath(
      projectContext.organizationId,
      projectContext.projectId,
      segment,
    );

  nav.project = [
    {
      label: "Project",
      items: [
        {
          label: "Overview",
          href: projectPath(
            projectContext.organizationId,
            projectContext.projectId,
          ),
          icon: Gauge,
          exact: true,
        },
        { label: "Services", href: href("services"), icon: Boxes },
        { label: "Deployments", href: href("deployments"), icon: CloudCog },
      ],
    },
    {
      label: "Compute",
      items: [
        { label: "Functions", href: href("functions"), icon: Zap },
        { label: "Sites", href: href("sites"), icon: Globe2 },
        { label: "Apps", href: href("apps"), icon: AppWindow },
        { label: "Agents", href: href("agents"), icon: Bot },
      ],
    },
    {
      label: "Data",
      items: [
        { label: "Databases", href: href("databases"), icon: Database },
        { label: "Storage", href: href("storage"), icon: Layers3 },
      ],
    },
    {
      label: "Platform",
      items: [
        { label: "Users", href: href("users"), icon: Users },
        { label: "Messaging", href: href("messaging"), icon: MessageSquare },
        { label: "Webhooks", href: href("webhooks"), icon: Webhook },
        { label: "API keys", href: href("api-keys"), icon: KeyRound },
      ],
    },
    {
      label: "Observability",
      items: [
        { label: "Logs", href: `${href("observability")}/logs`, icon: Cable },
        {
          label: "Traces",
          href: `${href("observability")}/traces`,
          icon: Activity,
        },
      ],
    },
    {
      label: "System",
      items: [
        {
          label: "Project settings",
          href: `${href("settings")}/project`,
          icon: Settings2,
        },
        { label: "Auth settings", href: href("auth"), icon: ShieldCheck },
      ],
    },
  ];
  return nav;
}
