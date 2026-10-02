import { DeploymentsView } from "@/features/deployments/deployments-view";

export default async function DeploymentsPage({
  params,
}: {
  params: Promise<{ organizationId: string; projectId: string }>;
}) {
  const { organizationId, projectId } = await params;
  return (
    <DeploymentsView organizationId={organizationId} projectId={projectId} />
  );
}
