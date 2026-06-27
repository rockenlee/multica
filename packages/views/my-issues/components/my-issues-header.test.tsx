import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Issue } from "@multica/core/types";
import { I18nProvider } from "@multica/core/i18n/react";
import { DEFAULT_ISSUE_SYNC_SETTINGS } from "@multica/core/issues";
import { myIssuesViewStore } from "@multica/core/issues/stores/my-issues-view-store";
import { ViewStoreProvider } from "@multica/core/issues/stores/view-store-context";
import enCommon from "../../locales/en/common.json";
import enIssues from "../../locales/en/issues.json";
import enMyIssues from "../../locales/en/my-issues.json";
import { MyIssuesHeader, MyIssuesSyncSettingsButton } from "./my-issues-header";

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/core/integrations", () => ({
  integrationKeys: {
    list: (wsId: string) => ["integrations", wsId, "list"] as const,
  },
  integrationsOptions: (wsId: string) => ({
    queryKey: ["integrations", wsId, "list"] as const,
    queryFn: async () => ({
      connections: [],
      accounts: [],
      project_bindings: [],
      issue_sync_settings: [],
      sync_events: [],
      credential_storage_enabled: false,
    }),
    enabled: !!wsId,
  }),
}));

vi.mock("@multica/core/api", () => ({
  api: {
    updateIntegrationIssueSyncSetting: vi.fn(async () => ({
      workspace_id: "ws-1",
      user_id: "u-1",
      provider: "gitlab",
      inbound_enabled: true,
      outbound_enabled: false,
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    })),
  },
}));

vi.mock("@multica/core/agents", () => ({
  agentTaskSnapshotOptions: () => ({
    queryKey: ["agent-task-snapshot", "ws-1"],
    queryFn: async () => [],
  }),
}));

const TEST_RESOURCES = {
  en: {
    common: enCommon,
    issues: enIssues,
    "my-issues": enMyIssues,
  },
};

function makeIssue(id: string, source?: string): Issue {
  return {
    id,
    workspace_id: "ws-1",
    number: 1,
    identifier: `MUL-${id}`,
    title: `Issue ${id}`,
    description: null,
    status: "todo",
    priority: "medium",
    assignee_type: null,
    assignee_id: null,
    creator_type: "member",
    creator_id: "u-1",
    parent_issue_id: null,
    project_id: null,
    position: 0,
    start_date: null,
    due_date: null,
    metadata: source ? { source_system: source } : {},
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function renderHeader() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <QueryClientProvider client={qc}>
        <ViewStoreProvider store={myIssuesViewStore}>
          <MyIssuesSyncSettingsButton />
          <MyIssuesHeader
            allIssues={[
              makeIssue("1", "gitlab"),
              makeIssue("2", "zentao"),
              makeIssue("3", "lark"),
            ]}
          />
        </ViewStoreProvider>
      </QueryClientProvider>
    </I18nProvider>,
  );
}

describe("MyIssuesHeader sync controls", () => {
  beforeEach(() => {
    myIssuesViewStore.setState({
      sourceFilters: [],
      syncSettings: DEFAULT_ISSUE_SYNC_SETTINGS,
      agentRunningFilter: false,
    });
  });

  it("shows issue sync settings for inbound and outbound channels", async () => {
    const user = userEvent.setup();
    renderHeader();

    await user.click(screen.getByRole("button", { name: /sync settings/i }));

    expect(screen.getByText("Issue sync settings")).toBeInTheDocument();
    expect(screen.getByText("Inbound")).toBeInTheDocument();
    expect(screen.getByText("Outbound")).toBeInTheDocument();
    expect(screen.getByText("GitLab")).toBeInTheDocument();
    expect(screen.getByText("ZenTao")).toBeInTheDocument();
    expect(screen.getByText("Lark")).toBeInTheDocument();
  });

  it("adds channel sync as a My Issues filter option", async () => {
    const user = userEvent.setup();
    renderHeader();

    await user.click(screen.getByRole("button", { name: /^filter$/i }));

    expect(screen.getByText("Channel sync")).toBeInTheDocument();
  });
});
