import { Suspense } from "react";
import { AdminTracesView } from "@/features/admin/admin-traces-view";
import { LoadingState } from "@/components/feedback/loading-state";

export default function AdminTracesPage() {
  return (
    <Suspense fallback={<LoadingState rows={6} />}>
      <AdminTracesView />
    </Suspense>
  );
}
