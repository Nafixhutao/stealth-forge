import { TracesView } from "@/features/observability/traces-view";

export default async function TracesPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  return <TracesView projectId={projectId} />;
}
