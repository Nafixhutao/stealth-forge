"use client";

import dynamic from "next/dynamic";
import { Skeleton } from "@/components/ui/skeleton";
import type { AdminMetricChartProps } from "./admin-metric-chart";

// ECharts is a large dependency only needed once metrics exist, so keep it out
// of the route's initial bundle and load it on demand.
export const AdminMetricChartLoader = dynamic<AdminMetricChartProps>(
  () =>
    import("./admin-metric-chart").then((module) => module.AdminMetricChart),
  {
    ssr: false,
    loading: () => <Skeleton className="h-72 w-full" />,
  },
);
