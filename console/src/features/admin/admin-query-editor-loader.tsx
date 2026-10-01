"use client";

import dynamic from "next/dynamic";
import { Skeleton } from "@/components/ui/skeleton";
import type { AdminQueryEditorProps } from "./admin-query-editor";

// CodeMirror is a large dependency only needed for the structured query input,
// so keep it out of the logs route's initial bundle.
export const AdminQueryEditorLoader = dynamic<AdminQueryEditorProps>(
  () =>
    import("./admin-query-editor").then((module) => module.AdminQueryEditor),
  {
    ssr: false,
    loading: () => <Skeleton className="h-[46px] w-full" />,
  },
);
