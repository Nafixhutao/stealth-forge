import { ProjectManagedDatabases } from "@/features/project/managed-databases";

/** The project's hosted MySQL database. */
export default async function ProjectMySQLPage({
  params,
}: {
  params: Promise<{ projectID: string }>;
}) {
  const { projectID } = await params;

  return <ProjectManagedDatabases projectId={projectID} />;
}
