import type { components } from "@/api/generated/schema";

export type DashboardRequest =
  components["schemas"]["CreateAdminDashboardRequest"];
export type Panel = {
  id?: string;
  type: string;
  title?: string;
  metric?: string;
  service?: string;
  level?: string;
  query?: string;
};
