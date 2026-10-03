import { MessagingView } from "@/features/messaging/messaging-view";

export default async function MessagingPage({
  params,
}: {
  params: Promise<{ projectId: string }>;
}) {
  const { projectId } = await params;
  return <MessagingView projectId={projectId} />;
}
