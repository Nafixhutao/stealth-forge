import { APIKeyDetailView } from "@/features/api-keys/api-key-detail-view";

export default async function APIKeyPage({
  params,
}: {
  params: Promise<{
    organizationId: string;
    projectId: string;
    keyId: string;
  }>;
}) {
  const { organizationId, projectId, keyId } = await params;
  return (
    <APIKeyDetailView
      organizationId={organizationId}
      projectId={projectId}
      keyId={keyId}
    />
  );
}
