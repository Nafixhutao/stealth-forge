import { OrganizationInvitationsView } from "@/features/organization/invitations-view";

export default async function InvitationsPage({
  params,
}: {
  params: Promise<{ organizationId: string }>;
}) {
  const { organizationId } = await params;
  return <OrganizationInvitationsView organizationId={organizationId} />;
}
