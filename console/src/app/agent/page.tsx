import { ApplicationShell } from "@/components/application-shell";
import { AgentChat } from "@/features/agent/agent-chat";

/**
 * The customer console's Agent surface: a conversation with one of the
 * account's agents. The shell supplies the navigation and the floating panel;
 * the page owns its own full-height chat layout inside it.
 *
 * `hasTopBar` hides the shell's floating nav opener on phones — the
 * conversation header carries the same control, so the page does not stack two
 * hamburgers in one corner.
 */
export default function AgentPage() {
  return (
    <ApplicationShell hasTopBar desktopHasTopBar={false} userConsole>
      <AgentChat />
    </ApplicationShell>
  );
}
