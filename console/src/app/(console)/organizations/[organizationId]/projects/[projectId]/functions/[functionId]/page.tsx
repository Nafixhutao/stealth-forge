import { FunctionDetailView } from "@/features/functions/function-detail-view";

export default async function FunctionPage({
  params,
}: {
  params: Promise<{
    organizationId: string;
    projectId: string;
    functionId: string;
  }>;
}) {
  const { organizationId, projectId, functionId } = await params;
  return (
    <FunctionDetailView
      organizationId={organizationId}
      projectId={projectId}
      functionId={functionId}
    />
  );
}
