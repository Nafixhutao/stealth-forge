import { AuthSettingsView } from "@/features/settings/auth-settings-view";

export default async function AuthPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  return <AuthSettingsView projectId={projectId} />;
}
