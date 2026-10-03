import { OrganizationPlanView } from "@/features/organization/plan-view";

export default async function PlanPage({
  params,
}: {
  params: Promise<{ organizationId: string }>;
}) {
  const { organizationId } = await params;
  return <OrganizationPlanView organizationId={organizationId} />;
}
