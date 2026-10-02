import { DatabaseDetailView } from "@/features/databases/database-detail-view";

export default async function DatabasePage({
  params,
}: {
  params: Promise<{
    organizationId: string;
    projectId: string;
    databaseId: string;
  }>;
}) {
  const { organizationId, projectId, databaseId } = await params;
  return (
    <DatabaseDetailView
      organizationId={organizationId}
      projectId={projectId}
      databaseId={databaseId}
    />
  );
}
