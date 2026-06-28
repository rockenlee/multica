import type { ReactNode } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";
import jaCommon from "../../locales/ja/common.json";
import jaSettings from "../../locales/ja/settings.json";

const mockInvalidate = vi.hoisted(() => vi.fn());
const mockListPersonalAccessTokens = vi.hoisted(() => vi.fn());
const mockDeleteIntegrationUserAccountById = vi.hoisted(() => vi.fn());
const mockUpsertIntegrationUserAccount = vi.hoisted(() => vi.fn());
const mockStartFeishuOAuth = vi.hoisted(() => vi.fn());
const mockZentaoLogin = vi.hoisted(() => vi.fn());

const integrationsRef = vi.hoisted(() => ({
  current: {
    connections: [] as unknown[],
    accounts: [] as unknown[],
    project_bindings: [],
    issue_sync_settings: [],
    sync_events: [],
    credential_storage_enabled: true,
  },
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: integrationsRef.current, isLoading: false }),
  useQueryClient: () => ({ invalidateQueries: mockInvalidate }),
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "workspace-1",
}));

vi.mock("@multica/core/integrations", () => ({
  integrationsOptions: (wsId: string) => ({ queryKey: ["integrations", wsId] }),
  integrationKeys: { list: (wsId: string) => ["integrations", wsId] },
}));

vi.mock("@multica/core/api", () => ({
  api: {
    listPersonalAccessTokens: mockListPersonalAccessTokens,
    deleteIntegrationUserAccountById: mockDeleteIntegrationUserAccountById,
    upsertIntegrationUserAccount: mockUpsertIntegrationUserAccount,
    startFeishuOAuth: mockStartFeishuOAuth,
    zentaoLogin: mockZentaoLogin,
  },
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

import { TokensTab } from "./tokens-tab";

const TEST_RESOURCES = {
  en: { common: enCommon, settings: enSettings },
  ja: { common: jaCommon, settings: jaSettings },
};

function I18nWrapper({ children }: { children: ReactNode }) {
  return (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      {children}
    </I18nProvider>
  );
}

function JaWrapper({ children }: { children: ReactNode }) {
  return (
    <I18nProvider locale="ja" resources={TEST_RESOURCES}>
      {children}
    </I18nProvider>
  );
}

function connection(overrides: Record<string, unknown>) {
  return {
    id: "connection-1",
    workspace_id: "workspace-1",
    provider: "gitlab",
    name: "GitLab",
    base_url: "https://gitlab.example.test",
    config: {},
    status: "active",
    sync_enabled: true,
    created_at: "2026-06-28T00:00:00Z",
    updated_at: "2026-06-28T00:00:00Z",
    ...overrides,
  };
}

function account(overrides: Record<string, unknown>) {
  return {
    id: "account-1",
    workspace_id: "workspace-1",
    connection_id: "connection-1",
    user_id: "user-1",
    account_key: "default",
    account_name: "Work account",
    external_user_id: null,
    external_username: "work-user",
    credential_configured: true,
    scopes: [],
    config: {},
    status: "active",
    sync_enabled: true,
    expires_at: null,
    last_used_at: null,
    last_error: null,
    created_at: "2026-06-28T00:00:00Z",
    updated_at: "2026-06-28T00:00:00Z",
    ...overrides,
  };
}

function resetFixtures() {
  vi.clearAllMocks();
  mockListPersonalAccessTokens.mockResolvedValue([]);
  mockDeleteIntegrationUserAccountById.mockResolvedValue(undefined);
  mockUpsertIntegrationUserAccount.mockResolvedValue(undefined);
  mockStartFeishuOAuth.mockResolvedValue({ authorize_url: "https://open.feishu.cn/oauth" });
  mockZentaoLogin.mockResolvedValue({ ok: true, account_key: "zentao" });
  integrationsRef.current = {
    connections: [],
    accounts: [],
    project_bindings: [],
    issue_sync_settings: [],
    sync_events: [],
    credential_storage_enabled: true,
  };
}

describe("TokensTab external accounts", () => {
  beforeEach(resetFixtures);

  it("uses an icon-only remove action and remove confirmation for stored external accounts", async () => {
    const user = userEvent.setup();
    integrationsRef.current = {
      ...integrationsRef.current,
      connections: [connection({ id: "gitlab-1", provider: "gitlab", name: "GitLab" })],
      accounts: [
        account({
          id: "gitlab-account-1",
          connection_id: "gitlab-1",
          account_name: "Work GitLab",
        }),
      ],
    };

    render(<TokensTab />, { wrapper: I18nWrapper });

    await screen.findByText("External CLI accounts");

    expect(screen.queryByRole("button", { name: /^Disconnect$/ })).toBeNull();

    const remove = screen.getByRole("button", { name: "Remove Work GitLab" });
    expect(remove.textContent?.trim()).toBe("");

    await user.click(remove);

    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.getByText("Remove this external account?")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove" })).toBeTruthy();
  });

  it("deduplicates only repeated Feishu OAuth user accounts", async () => {
    integrationsRef.current = {
      ...integrationsRef.current,
      connections: [
        connection({
          id: "feishu-1",
          provider: "feishu",
          name: "Feishu",
          base_url: "https://open.feishu.cn",
          config: { app_id: "cli_a942b38e13f8dbc7" },
        }),
        connection({ id: "gitlab-1", provider: "gitlab", name: "GitLab" }),
        connection({ id: "zentao-1", provider: "zentao", name: "ZenTao" }),
      ],
      accounts: [
        account({
          id: "feishu-oauth-1",
          connection_id: "feishu-1",
          account_key: "feishu-user-oauth",
          account_name: "Feishu OAuth",
          external_user_id: "ou_elon",
          external_username: "Elon",
        }),
        account({
          id: "feishu-oauth-2",
          connection_id: "feishu-1",
          account_key: "feishu_user_oauth",
          account_name: "Feishu OAuth duplicate",
          external_user_id: "ou_elon",
          external_username: "Elon",
        }),
        account({
          id: "feishu-app-1",
          connection_id: "feishu-1",
          account_key: "litianyi-feishu-app",
          account_name: "Feishu app credential",
          external_user_id: null,
          external_username: "cli_a942b38e13f8dbc7",
        }),
        account({
          id: "gitlab-account-1",
          connection_id: "gitlab-1",
          account_key: "default",
          account_name: "GitLab token A",
        }),
        account({
          id: "gitlab-account-2",
          connection_id: "gitlab-1",
          account_key: "secondary",
          account_name: "GitLab token B",
        }),
        account({
          id: "zentao-account-1",
          connection_id: "zentao-1",
          account_key: "admin",
          account_name: "ZenTao admin",
        }),
        account({
          id: "zentao-account-2",
          connection_id: "zentao-1",
          account_key: "tester",
          account_name: "ZenTao tester",
        }),
      ],
    };

    render(<TokensTab />, { wrapper: I18nWrapper });

    await screen.findByText("External CLI accounts");

    expect(screen.getByText("Feishu OAuth")).toBeTruthy();
    expect(screen.queryByText("Feishu OAuth duplicate")).toBeNull();
    expect(screen.getByText("Feishu app credential")).toBeTruthy();
    expect(screen.getByText("GitLab token A")).toBeTruthy();
    expect(screen.getByText("GitLab token B")).toBeTruthy();
    expect(screen.getByText("ZenTao admin")).toBeTruthy();
    expect(screen.getByText("ZenTao tester")).toBeTruthy();

    await waitFor(() => {
      expect(mockListPersonalAccessTokens).toHaveBeenCalled();
    });
  });

  it("localizes the external account remove aria/tooltip for non-English locales", async () => {
    integrationsRef.current = {
      ...integrationsRef.current,
      connections: [connection({ id: "gitlab-1", provider: "gitlab", name: "GitLab" })],
      accounts: [
        account({
          id: "gitlab-account-1",
          connection_id: "gitlab-1",
          account_name: "Work GitLab",
        }),
      ],
    };

    render(<TokensTab />, { wrapper: JaWrapper });

    // The trash button aria must follow the active locale, not stay English.
    expect(await screen.findByRole("button", { name: "Work GitLab を削除" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Remove Work GitLab" })).toBeNull();
  });
});
