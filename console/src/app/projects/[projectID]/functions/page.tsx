import { ProjectFunctions } from "@/features/project/functions";

/** A project's functions. */
export default async function ProjectFunctionsPage({
  params,
}: {
  params: Promise<{ projectID: string }>;
}) {
  const { projectID } = await params;

  return <ProjectFunctions projectId={projectID} />;
}
