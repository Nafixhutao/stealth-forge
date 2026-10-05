import type { components } from "@/api/generated/schema";
import { formatRelative } from "@/lib/format";
import type { Incident, IncidentUpdate } from "../types/incidents";

/**
 * Adapters that translate real Stealth API payloads into the shapes the
 * redesigned admin views render. The admin components were authored against a
 * local mock model, so each surface maps its API response here rather than
 * leaking the wire shape into presentation code.
 */

type ApiIncident = components["schemas"]["AdminIncident"];

function incidentDuration(
  startedAt: string,
  resolvedAt: string | null | undefined,
): string {
  const start = new Date(startedAt).getTime();
  const end = resolvedAt ? new Date(resolvedAt).getTime() : Date.now();
  const minutes = Math.max(0, Math.round((end - start) / 60_000));
  const label =
    minutes < 60
      ? `${minutes}m`
      : `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
  return resolvedAt ? label : `Ongoing for ${label}`;
}

function toIncidentUpdate(
  event: ApiIncident["events"][number],
): IncidentUpdate {
  return {
    time: formatRelative(event.created_at),
    kind: event.kind,
    message: event.message,
  };
}

/** API incident → the redesigned board's incident model. */
export function toIncident(item: ApiIncident): Incident {
  return {
    id: item.id,
    title: item.title,
    severity: item.severity,
    services: item.services,
    status: item.status,
    startedAt: formatRelative(item.started_at),
    duration: incidentDuration(item.started_at, item.resolved_at),
    updates: item.events.map(toIncidentUpdate),
  };
}
