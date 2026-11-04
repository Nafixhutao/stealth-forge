import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AgentChat } from "@/features/agent/agent-chat";

// The page is verified through its own contract: what it shows per data state,
// and what the composer does. The hooks are stubbed, so the assertions are
// about this component rather than the network.
const useAgents = vi.fn();
const useAgentRuns = vi.fn();
const useAgentCatalog = vi.fn();
const useAgentArtifacts = vi.fn();
const mutateAsync = vi.fn();
const push = vi.fn();

vi.mock("@/api/queries/agents", () => ({
  useAgents: () => useAgents(),
  useAgentRuns: (id: string | undefined) => useAgentRuns(id),
  useAgentCatalog: () => useAgentCatalog(),
  useAgentArtifacts: (id: string | undefined) => useAgentArtifacts(id),
}));

vi.mock("@/api/mutations/agents", () => ({
  useCreateAgentRun: () => ({ mutateAsync, isPending: false }),
}));

vi.mock("@/api/queries/account", () => ({
  useCurrentAccount: () => ({
    data: {
      account: {
        id: "acc-1",
        email: "operator@example.com",
        display_name: "Operator",
        instance_role: null,
      },
    },
  }),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn(), prefetch: vi.fn() }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("@/components/application-shell", () => ({
  useApplicationShell: () => ({ openSidebar: vi.fn() }),
}));

const agent = {
  id: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  project_id: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
  project_name: "acme",
  name: "Greeting from Acme",
  description: "",
  role: "General",
  status: "idle",
  branch: "main",
  provider: "openai",
  model: "gpt-5",
  current_task: null,
  last_active_at: null,
  tools: ["Read files", "Run tests"],
  instructions: null,
  created_by_account_id: null,
  created_at: "2026-10-01T00:00:00Z",
  updated_at: "2026-10-01T00:00:00Z",
};

function run(overrides: Record<string, unknown> = {}) {
  return {
    id: "cccccccc-cccc-cccc-cccc-cccccccccccc",
    agent_id: agent.id,
    project_id: agent.project_id,
    prompt: "hi",
    status: "queued",
    output_text: null,
    error_message: null,
    steps: [],
    changes: [],
    created_by_account_id: null,
    queued_at: "2026-11-01T11:32:00Z",
    started_at: null,
    finished_at: null,
    created_at: "2026-11-01T11:32:00Z",
    updated_at: "2026-11-01T11:32:00Z",
    ...overrides,
  };
}

function query(overrides: Record<string, unknown> = {}) {
  return {
    data: undefined,
    isPending: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  useAgents.mockReturnValue(
    query({ data: { agents: [agent], can_manage: true } }),
  );
  useAgentRuns.mockReturnValue(
    query({ data: { runs: [run()], can_manage: true } }),
  );
  useAgentCatalog.mockReturnValue(
    query({
      data: {
        providers: [],
        roles: ["General"],
        tools: ["Read files"],
        execution: { mode: "queue_only", ready: false, message: "" },
      },
    }),
  );
  useAgentArtifacts.mockReturnValue(
    query({
      data: { artifacts: [], pagination: { limit: 50, next_cursor: null } },
    }),
  );
  mutateAsync.mockResolvedValue({ run: run(), can_manage: true });
});

describe("AgentChat", () => {
  it("opens on the New chat header and draws the active thread", () => {
    render(<AgentChat />);

    expect(
      screen.getByRole("button", { name: "Agent chat menu" }),
    ).toHaveTextContent("New chat");
    expect(screen.getByText("hi")).toBeInTheDocument();
  });

  it("explains that a queued run has no worker instead of faking a reply", () => {
    render(<AgentChat />);

    expect(
      screen.getByText(/so the run waits in the queue/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/so replies stay pending/i)).toBeInTheDocument();
  });

  it("renders the agent's reply when the run carries one", () => {
    useAgentRuns.mockReturnValue(
      query({
        data: {
          runs: [run({ status: "succeeded", output_text: "Hello there." })],
          can_manage: true,
        },
      }),
    );

    render(<AgentChat />);

    expect(screen.getByText("Hello there.")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Copy reply" }),
    ).toBeInTheDocument();
  });

  it("queues the typed prompt and clears the box", async () => {
    render(<AgentChat />);

    const box = screen.getByLabelText(/ask agent/i);
    fireEvent.change(box, { target: { value: "ship it" } });
    fireEvent.keyDown(box, { key: "Enter" });

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith("ship it"));
    expect(box).toHaveValue("");
  });

  it("keeps a newline on Shift+Enter instead of sending", () => {
    render(<AgentChat />);

    const box = screen.getByLabelText(/ask agent/i);
    fireEvent.change(box, { target: { value: "two lines" } });
    fireEvent.keyDown(box, { key: "Enter", shiftKey: true });

    expect(mutateAsync).not.toHaveBeenCalled();
  });

  it("refuses to send when the member cannot manage runs", () => {
    useAgentRuns.mockReturnValue(
      query({ data: { runs: [], can_manage: false } }),
    );

    render(<AgentChat />);

    expect(screen.getByLabelText(/ask agent/i)).toBeDisabled();
    expect(
      screen.getByText(/only an owner or admin of this agent/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send prompt" })).toBeDisabled();
  });

  it("opens on the composer instead of sending the reader to a project", () => {
    useAgents.mockReturnValue(
      query({ data: { agents: [], can_manage: false } }),
    );

    render(<AgentChat />);

    expect(screen.getByLabelText(/ask agent/i)).toBeInTheDocument();
    expect(screen.getByText("Get started")).toBeInTheDocument();
    expect(screen.getByText("Greeting from Operator")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Go to projects" }),
    ).toBeNull();
    expect(screen.queryByText("No agent yet")).toBeNull();
  });

  it("surfaces a failed read with a retry", () => {
    useAgents.mockReturnValue(
      query({ isError: true, error: new Error("boom") }),
    );

    render(<AgentChat />);

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Unable to load agents",
    );
  });

  it("shows the artifact panel and download links when a run produced files", () => {
    useAgentArtifacts.mockReturnValue(
      query({
        data: {
          artifacts: [
            {
              id: "artifact-1",
              project_id: "project-1",
              agent_id: agent.id,
              run_id: "run-1",
              kind: "document",
              title: "letter.pdf",
              version: 1,
              mime_type: "application/pdf",
              size_bytes: 1234,
              created_at: "2026-11-02T00:00:00Z",
            },
          ],
          pagination: { limit: 50, next_cursor: null },
        },
      }),
    );

    render(<AgentChat />);

    expect(screen.getByText("Artifact")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "PDF" })).toHaveAttribute(
      "href",
      expect.stringContaining("/artifacts/artifact-1/download"),
    );
  });
});
