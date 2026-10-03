import { AgentsView } from "@/features/agents/agents-view";

export default async function AgentsPage({
  params,
}: {
  params: Promise<{ organizationId: string; projectId: string }>;
}) {
  const { organizationId, projectId } = await params;
  return <AgentsView organizationId={organizationId} projectId={projectId} />;
}
