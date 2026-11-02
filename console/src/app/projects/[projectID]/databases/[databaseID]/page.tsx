import { DatabaseStudio } from "@/features/project/database-studio";

/** One database's tables, columns, and rows. */
export default async function ProjectDatabasePage({
  params,
}: {
  params: Promise<{ projectID: string; databaseID: string }>;
}) {
  const { projectID, databaseID } = await params;

  return <DatabaseStudio projectId={projectID} databaseId={databaseID} />;
}
