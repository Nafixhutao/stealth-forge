import { OrganizationTracesView } from "@/features/organization/traces-view";

export default async function OrganizationTracesPage({
  params,
}: {
  params: Promise<{ organizationId: string }>;
}) {
  const { organizationId } = await params;
  return <OrganizationTracesView organizationId={organizationId} />;
}
