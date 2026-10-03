import { OrganizationAuditView } from "@/features/organization/audit-view";

export default async function AuditPage({
  params,
}: {
  params: Promise<{ organizationId: string }>;
}) {
  const { organizationId } = await params;
  return <OrganizationAuditView organizationId={organizationId} />;
}
