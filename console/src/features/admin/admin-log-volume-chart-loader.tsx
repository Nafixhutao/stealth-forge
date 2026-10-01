"use client";

import dynamic from "next/dynamic";
import { Skeleton } from "@/components/ui/skeleton";
import type { AdminLogVolumeChartProps } from "./admin-log-volume-chart";

// ECharts is a large dependency, so load the volume chart on demand instead of
// bundling it with the errors route.
export const AdminLogVolumeChartLoader = dynamic<AdminLogVolumeChartProps>(
  () =>
    import("./admin-log-volume-chart").then(
      (module) => module.AdminLogVolumeChart,
    ),
  {
    ssr: false,
    loading: () => <Skeleton className="h-48 w-full" />,
  },
);
