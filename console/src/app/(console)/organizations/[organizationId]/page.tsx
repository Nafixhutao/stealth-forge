import { OrganizationOverviewView } from "@/features/organization/overview-view";

export default async function OrganizationPage({
  params,
}: {
  params: Promise<{ organizationId: string }>;
}) {
  const { organizationId } = await params;
  return <OrganizationOverviewView organizationId={organizationId} />;
}
