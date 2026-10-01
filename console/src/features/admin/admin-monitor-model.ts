import {
  CreateAdminMonitorRequestKind,
  CreateAdminMonitorRequestMethod,
  type components,
} from "@/api/generated/schema";

export type MonitorKind =
  components["schemas"]["CreateAdminMonitorRequest"]["kind"];
export type CreateMonitorRequest =
  components["schemas"]["CreateAdminMonitorRequest"];

export const monitorKinds: Array<{ value: MonitorKind; label: string }> = [
  { value: CreateAdminMonitorRequestKind.http, label: "HTTP / HTTPS" },
  { value: CreateAdminMonitorRequestKind.tcp, label: "TCP" },
  { value: CreateAdminMonitorRequestKind.dns, label: "DNS" },
  { value: CreateAdminMonitorRequestKind.tls, label: "TLS certificate" },
  { value: CreateAdminMonitorRequestKind.heartbeat, label: "Heartbeat" },
];

export const initialForm: CreateMonitorRequest = {
  name: "",
  kind: CreateAdminMonitorRequestKind.http,
  target: "",
  interval_seconds: 60,
  timeout_ms: 5000,
  enabled: true,
  method: CreateAdminMonitorRequestMethod.GET,
  expected_status: 200,
};
