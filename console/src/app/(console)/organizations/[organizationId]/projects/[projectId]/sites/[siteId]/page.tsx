import { SiteDetailView } from "@/features/sites/site-detail-view";

export default async function SitePage({
  params,
}: {
  params: Promise<{
    organizationId: string;
    projectId: string;
    siteId: string;
  }>;
}) {
  const { organizationId, projectId, siteId } = await params;
  return (
    <SiteDetailView
      organizationId={organizationId}
      projectId={projectId}
      siteId={siteId}
    />
  );
}
