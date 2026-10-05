export type IncidentSeverity = "critical" | "warning" | "info";

export type IncidentStatus = "investigating" | "identified" | "monitoring" | "resolved";

/** A timestamped status update on an incident timeline. */
export interface IncidentUpdate {
  time: string;
  status: IncidentStatus;
  message: string;
}

/** A platform incident with severity, blast radius, and timeline. */
export interface Incident {
  id: string;
  title: string;
  severity: IncidentSeverity;
  services: string[];
  status: IncidentStatus;
  startedAt: string;
  /** Pre-formatted duration, "12m" / "1h 42m" / "Ongoing for 12m". */
  duration: string;
  updates: IncidentUpdate[];
}
