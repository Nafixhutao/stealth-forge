import { APIKeysView } from "@/features/api-keys/api-keys-view";

export default async function APIKeysPage({
  params,
}: {
  params: Promise<{ organizationId: string; projectId: string }>;
}) {
  const { organizationId, projectId } = await params;
  return <APIKeysView organizationId={organizationId} projectId={projectId} />;
}
