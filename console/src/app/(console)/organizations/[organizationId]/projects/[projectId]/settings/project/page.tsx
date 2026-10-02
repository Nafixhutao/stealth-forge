import { ProjectSettingsView } from "@/features/settings/project-settings-view";

export default async function ProjectSettingsPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  return <ProjectSettingsView projectId={projectId} />;
}
