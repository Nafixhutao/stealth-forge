import { ProjectSites } from "@/features/project/sites";

/** A project's sites. */
export default async function ProjectSitesPage({
  params,
}: {
  params: Promise<{ projectID: string }>;
}) {
  const { projectID } = await params;

  return <ProjectSites projectId={projectID} />;
}
