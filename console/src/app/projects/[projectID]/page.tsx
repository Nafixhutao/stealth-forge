import { ProjectOverview } from "@/features/project/project-overview";

/** A project's Overview: what it holds, and what it has served. */
export default async function ProjectOverviewPage({
  params,
}: {
  params: Promise<{ projectID: string }>;
}) {
  const { projectID } = await params;

  return <ProjectOverview projectId={projectID} />;
}
