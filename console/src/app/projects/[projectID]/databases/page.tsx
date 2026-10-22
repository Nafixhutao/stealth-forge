import { ProjectDatabases } from "@/features/project/databases";

/** A project's databases. */
export default async function ProjectDatabasesPage({
  params,
}: {
  params: Promise<{ projectID: string }>;
}) {
  const { projectID } = await params;

  return <ProjectDatabases projectId={projectID} />;
}
