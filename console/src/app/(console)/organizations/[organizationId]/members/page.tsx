import { OrganizationMembersView } from "@/features/organization/members-view";

export default async function MembersPage({
  params,
}: {
  params: Promise<{ organizationId: string }>;
}) {
  const { organizationId } = await params;
  return <OrganizationMembersView organizationId={organizationId} />;
}
