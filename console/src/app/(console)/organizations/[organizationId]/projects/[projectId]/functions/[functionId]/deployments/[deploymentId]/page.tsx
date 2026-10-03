import { FunctionDeploymentView } from "@/features/functions/function-detail-view";

export default async function FunctionDeploymentPage({
  params,
}: {
  params: Promise<{
    organizationId: string;
    projectId: string;
    functionId: string;
    deploymentId: string;
  }>;
}) {
  const { organizationId, projectId, functionId, deploymentId } = await params;
  return (
    <FunctionDeploymentView
      organizationId={organizationId}
      projectId={projectId}
      functionId={functionId}
      deploymentId={deploymentId}
    />
  );
}
