import { UsersView } from "@/features/users/users-view";

export default async function UsersPage({
  params,
}: {
  params: Promise<{ organizationId: string; projectId: string }>;
}) {
  const { organizationId, projectId } = await params;
  return <UsersView organizationId={organizationId} projectId={projectId} />;
}
