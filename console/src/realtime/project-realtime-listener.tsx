"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { apiUrl } from "@/api/client";
import { applyCacheChanges } from "@/api/cache-coherence";
import {
  eventTypesForProjectStream,
  realtimeCacheChanges,
  type RealtimeNotification,
} from "@/realtime/invalidation";

export function ProjectRealtimeListener({
  projectId,
}: Readonly<{ projectId: string | undefined }>) {
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!projectId || typeof EventSource === "undefined") return;
    // The server treats an absent `events` filter as "all events" and rejects
    // more than 64 names, so never send the full subscription list. Individual
    // listeners below still narrow which notifications become cache changes.
    const source = new EventSource(
      apiUrl(`/v1/projects/${encodeURIComponent(projectId)}/realtime`),
      { withCredentials: true },
    );
    const listeners = eventTypesForProjectStream().map((eventType) => {
      const listener = (message: MessageEvent<string>) => {
        try {
          const event = JSON.parse(message.data) as RealtimeNotification;
          void applyCacheChanges(
            queryClient,
            realtimeCacheChanges(projectId, event),
          );
        } catch {
          // A malformed notification cannot become client state. Existing
          // bounded polling remains the recovery path.
        }
      };
      source.addEventListener(eventType, listener);
      return { eventType, listener };
    });
    return () => {
      for (const { eventType, listener } of listeners) {
        source.removeEventListener(eventType, listener);
      }
      source.close();
    };
  }, [projectId, queryClient]);

  return null;
}
