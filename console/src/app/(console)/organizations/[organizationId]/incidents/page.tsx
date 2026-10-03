import { OrganizationIncidentsView } from "@/features/organization/incidents-view";

export default async function IncidentsPage({
  params,
}: {
  params: Promise<{ organizationId: string }>;
}) {
  const { organizationId } = await params;
  return <OrganizationIncidentsView organizationId={organizationId} />;
}
