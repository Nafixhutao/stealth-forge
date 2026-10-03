import { Suspense } from "react";
import { AdminLogsView } from "@/features/admin/admin-logs-view";
import { LoadingState } from "@/components/feedback/loading-state";

export default function AdminLogsPage() {
  return (
    <Suspense fallback={<LoadingState rows={6} />}>
      <AdminLogsView />
    </Suspense>
  );
}
