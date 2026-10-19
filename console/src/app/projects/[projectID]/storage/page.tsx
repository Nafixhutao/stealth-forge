import { ProjectStorage } from "@/features/project/storage";

/** A project's storage buckets. */
export default async function ProjectStoragePage({
  params,
}: {
  params: Promise<{ projectID: string }>;
}) {
  const { projectID } = await params;

  return <ProjectStorage projectId={projectID} />;
}
