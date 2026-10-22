import { SiteDetail } from "@/features/project/site-detail";

/**
 * One Site: its live preview, its releases, and the source of a release.
 *
 * The id is read from the route so the page stays a server component and the
 * detail surface owns its own client boundary.
 */
export default async function SiteDetailPage({
  params,
}: {
  params: Promise<{ projectID: string; siteID: string }>;
}) {
  const { projectID, siteID } = await params;
  return <SiteDetail projectId={projectID} siteId={siteID} />;
}
