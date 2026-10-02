import { SitesView } from "@/features/sites/sites-view";

export default async function SitesPage({
  params,
}: {
  params: Promise<{ organizationId: string; projectId: string }>;
}) {
  const { organizationId, projectId } = await params;
  return <SitesView organizationId={organizationId} projectId={projectId} />;
}
