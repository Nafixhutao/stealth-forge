"use client";

import dynamic from "next/dynamic";
import { LoadingState } from "@/components/feedback/loading-state";
import { Skeleton } from "@/components/ui/skeleton";

// React Flow is a large dependency, so load the service map on demand rather
// than bundling it with the admin shell.
const AdminServiceMapView = dynamic(
  () =>
    import("./admin-service-map-view").then(
      (module) => module.AdminServiceMapView,
    ),
  {
    ssr: false,
    loading: () => (
      <LoadingState
        label="Loading service map…"
        className="h-[calc(100vh-15rem)] min-h-[520px] w-full"
      >
        <Skeleton className="h-full w-full rounded-2xl" />
      </LoadingState>
    ),
  },
);

export function AdminServiceMapLoader() {
  return <AdminServiceMapView />;
}
