"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Cloud,
  ExternalLink,
  GitBranch,
  GitCommitHorizontal,
  GitMerge,
  Kanban,
  Link2,
  PanelRight,
  Save,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { LarkTab } from "./lark-tab";
import { ComposioTab } from "./composio-tab";
import { SlackTab } from "./slack-tab";
import { VCSTab } from "./vcs-tab";
import { api, ApiError } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { integrationsOptions, integrationKeys } from "@multica/core/integrations";
import { useWorkspaceId } from "@multica/core/hooks";
import type {
  IntegrationConnection,
  IntegrationProvider,
  IntegrationSyncEvent,
  IntegrationUserAccount,
} from "@multica/core/types";
import { memberListOptions } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { composioToolkitsOptions } from "@multica/core/composio";
import { useConfigStore, useFeatureEnabled } from "@multica/core/config";
import { COMPOSIO_MCP_APPS_FLAG } from "@multica/core/feature-flags";
import { useT } from "../../i18n";
import { useNavigation } from "../../navigation";
import { SettingsSection, SettingsTab } from "./settings-layout";

type ProviderDefinition = {
  provider: IntegrationProvider;
  title: string;
  icon: ReactNode;
};

type ProviderCopy = {
  title: string;
  subtitle: string;
  baseUrlPlaceholder: string;
  accountPlaceholder: string;
  credentialPlaceholder: string;
  connectionHelp: string;
  accountHelp: string;
  credentialHelp: string;
};

const PROVIDERS: ProviderDefinition[] = [
  { provider: "gitlab", title: "GitLab", icon: <GitBranch className="h-4 w-4" /> },
  { provider: "zentao", title: "ZenTao", icon: <Kanban className="h-4 w-4" /> },
  { provider: "feishu", title: "Feishu", icon: <Cloud className="h-4 w-4" /> },
];

// Integrations is the umbrella tab for third-party platform connections.
// GitHub has its own top-level tab (see github-tab.tsx); everything else
// — currently Lark, Composio, Slack, and the self-hosted Git providers (Forgejo /
// Gitea / GitLab), with Linear etc. to follow — lives in here under its own
// section heading so additional integrations slot in without changing the IA.
// IntegrationsTab is just the host; each integration owns its own description
// and install flow.
export function IntegrationsTab() {
  const { t } = useT("settings");
  const composioEnabled = useFeatureEnabled(COMPOSIO_MCP_APPS_FLAG, false);
  const composioToolkits = useQuery({
    ...composioToolkitsOptions(),
    enabled: composioEnabled,
  });
  const composioUnconfigured =
    composioToolkits.error instanceof ApiError && composioToolkits.error.status === 503;
  const vcsAvailable = useConfigStore((state) => state.vcsIntegrationAvailable);

  return (
    <SettingsTab title={t(($) => $.page.tabs.integrations)}>
      <SettingsSection title={t(($) => $.enterprise_integrations.center_title)}>
        <IntegrationCenterOverview />
      </SettingsSection>
      <SettingsSection title={t(($) => $.lark.section_title)}>
        <LarkTab />
      </SettingsSection>
      {composioEnabled && !composioUnconfigured && (
        <SettingsSection title={t(($) => $.composio.section_title)}>
          <ComposioTab />
        </SettingsSection>
      )}
      <SettingsSection title={t(($) => $.slack.section_title)}>
        <SlackTab />
      </SettingsSection>
      {vcsAvailable && (
        <SettingsSection title={t(($) => $.vcs.section_title)}>
          <VCSTab />
        </SettingsSection>
      )}
    </SettingsTab>
  );
}

export function GitLabIntegrationTab() {
  return <ProviderSettingsPage providers={["gitlab"]} />;
}

export function GitLabFeatureCard() {
  return <ProviderFeatureToggleCard provider="gitlab" />;
}

export function GitLabConnectionCard() {
  return <ProviderConnectionFormCard provider="gitlab" />;
}

const GITLAB_FEATURE_KEYS = [
  "mr_sidebar_enabled",
  "auto_link_mrs_enabled",
  "co_authored_by_enabled",
  "status_advance_enabled",
] as const;
type GitLabFeatureKey = (typeof GITLAB_FEATURE_KEYS)[number];

export function GitLabFeaturesCard() {
  const { t } = useT("settings");
  const qc = useQueryClient();
  const { wsId, connection, canManage } = useProviderConnectionContext("gitlab");
  const [savingKey, setSavingKey] = useState<GitLabFeatureKey | null>(null);
  const enabled = connection?.sync_enabled === true;
  const config = (connection?.config ?? {}) as Record<string, unknown>;
  const flag = (key: GitLabFeatureKey) => config[key] === true;
  const rowDisabled = (key: GitLabFeatureKey) =>
    !canManage || !enabled || savingKey === key;

  async function persistFlag(key: GitLabFeatureKey, next: boolean) {
    if (!canManage || !connection || savingKey) return;
    setSavingKey(key);
    try {
      await api.updateIntegrationConnection(wsId, connection.id, {
        provider: "gitlab",
        name: connection.name || "GitLab",
        base_url: connection.base_url,
        status: connection.status || "active",
        sync_enabled: connection.sync_enabled === true,
        config: { ...(connection.config ?? {}), [key]: next },
      });
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($) => $.enterprise_integrations.connection_save_failed));
    } finally {
      setSavingKey(null);
    }
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <GitLabFeatureRow id="gitlab-mr-sidebar" icon={<PanelRight className="h-4 w-4" />} label={t(($) => $.github.gitlab_feature_mr_sidebar_label)} description={t(($) => $.github.gitlab_feature_mr_sidebar_description)} checked={flag("mr_sidebar_enabled")} disabled={rowDisabled("mr_sidebar_enabled")} onCheckedChange={(value) => void persistFlag("mr_sidebar_enabled", value)} />
        <GitLabFeatureRow id="gitlab-auto-link" icon={<Link2 className="h-4 w-4" />} label={t(($) => $.github.gitlab_feature_auto_link_label)} description={t(($) => $.github.gitlab_feature_auto_link_description)} checked={flag("auto_link_mrs_enabled")} disabled={rowDisabled("auto_link_mrs_enabled")} onCheckedChange={(value) => void persistFlag("auto_link_mrs_enabled", value)} />
        <GitLabFeatureRow id="gitlab-co-author" icon={<GitCommitHorizontal className="h-4 w-4" />} label={t(($) => $.github.gitlab_feature_co_author_label)} description={t(($) => $.github.gitlab_feature_co_author_description)} checked={flag("co_authored_by_enabled")} disabled={rowDisabled("co_authored_by_enabled")} onCheckedChange={(value) => void persistFlag("co_authored_by_enabled", value)} />
        <GitLabFeatureRow id="gitlab-status-advance" icon={<GitMerge className="h-4 w-4" />} label={t(($) => $.github.gitlab_feature_status_advance_label)} description={t(($) => $.github.gitlab_feature_status_advance_description)} checked={flag("status_advance_enabled")} disabled={rowDisabled("status_advance_enabled")} onCheckedChange={(value) => void persistFlag("status_advance_enabled", value)} />
      </CardContent>
    </Card>
  );
}

function GitLabFeatureRow({ id, icon, label, description, checked, disabled, onCheckedChange }: {
  id: string;
  icon: ReactNode;
  label: string;
  description: string;
  checked: boolean;
  disabled: boolean;
  onCheckedChange: (value: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex items-start gap-3">
        <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">{icon}</div>
        <div className="space-y-1">
          <Label htmlFor={id} className="text-body font-medium">{label}</Label>
          <p className="text-body text-muted-foreground">{description}</p>
        </div>
      </div>
      <Switch id={id} checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} />
    </div>
  );
}

export function ZenTaoIntegrationTab() {
  return <ProviderSettingsPage providers={["zentao"]} />;
}

export function FeishuIntegrationTab() {
  return <ProviderSettingsPage providers={["feishu"]} />;
}

function ProviderSettingsPage({ providers }: { providers: IntegrationProvider[] }) {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const user = useAuthStore((state) => state.user);
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data, isLoading } = useQuery(integrationsOptions(wsId));
  const currentMember = members.find((member) => member.user_id === user?.id) ?? null;
  const canManage = currentMember?.role === "owner" || currentMember?.role === "admin";
  const connections = data?.connections ?? [];
  const providerCopy = useProviderCopy();
  const activeDefinitions = PROVIDERS.filter((definition) => providers.includes(definition.provider));
  const providerTitle = activeDefinitions.length === 1
    ? providerCopy[activeDefinitions[0]!.provider].title
    : t(($) => $.enterprise_integrations.title);

  return (
    <SettingsTab title={providerTitle}>
      <SettingsSection title={providerTitle}>
        {isLoading ? (
          <div className="text-body text-muted-foreground">{t(($) => $.lark.loading)}</div>
        ) : (
          <div className="space-y-4">
            {activeDefinitions.map((definition) => (
              <ProviderCard
                key={definition.provider}
                definition={definition}
                copy={providerCopy[definition.provider]}
                connection={connections.find((item) => item.provider === definition.provider) ?? null}
                canManage={canManage}
              />
            ))}
          </div>
        )}
      </SettingsSection>
    </SettingsTab>
  );
}

function useProviderConnectionContext(provider: IntegrationProvider) {
  const wsId = useWorkspaceId();
  const user = useAuthStore((s) => s.user);
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data } = useQuery(integrationsOptions(wsId));
  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManage =
    currentMember?.role === "owner" || currentMember?.role === "admin";
  const providerCopy = useProviderCopy();
  const definition = PROVIDERS.find((item) => item.provider === provider);
  const connection =
    data?.connections.find((item) => item.provider === provider) ?? null;
  return {
    wsId,
    definition,
    copy: providerCopy[provider],
    connection,
    canManage,
  };
}

function ProviderFeatureToggleCard({ provider }: { provider: IntegrationProvider }) {
  const { t } = useT("settings");
  const qc = useQueryClient();
  const { wsId, definition, connection, canManage } =
    useProviderConnectionContext(provider);
  const syncScopes = useProviderSyncScopes(provider);
  const [saving, setSaving] = useState(false);
  if (!definition) return null;

  const providerTitle = definition.title;
  const enabled = connection?.sync_enabled === true;
  const description = !connection
    ? t(($) => $.github.gitlab_master_description_unconnected)
    : enabled
      ? t(($) => $.github.gitlab_master_description_on)
      : t(($) => $.github.gitlab_master_description_off);

  async function persistEnabled(next: boolean) {
    if (!canManage || !connection || saving) return;
    setSaving(true);
    try {
      await api.updateIntegrationConnection(wsId, connection.id, {
        provider,
        name: connection.name || providerTitle,
        base_url: connection.base_url,
        status: connection.status || "active",
        sync_enabled: next,
        config: {
          sync_scopes: syncScopes.map((scope) => scope.key),
          ...(connection.config ?? {}),
        },
      });
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
      toast.success(t(($) => $.enterprise_integrations.connection_saved));
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.enterprise_integrations.connection_save_failed),
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <Card>
      <CardContent>
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-3">
            <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
              {definition.icon}
            </div>
            <div className="space-y-1">
              <Label htmlFor={`${provider}-master`} className="text-body font-medium">
                {t(($) => $.github.gitlab_section_master)}
              </Label>
              <p className="text-body text-muted-foreground">
                {description}
              </p>
            </div>
          </div>
          <Switch
            id={`${provider}-master`}
            checked={enabled}
            onCheckedChange={(value) => {
              void persistEnabled(value);
            }}
            disabled={!canManage || !connection || saving}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function ProviderConnectionFormCard({ provider }: { provider: IntegrationProvider }) {
  const { t } = useT("settings");
  const qc = useQueryClient();
  const { wsId, definition, copy, connection, canManage } =
    useProviderConnectionContext(provider);
  const syncScopes = useProviderSyncScopes(provider);
  const [baseUrl, setBaseUrl] = useState("");
  const [savingConnection, setSavingConnection] = useState(false);

  useEffect(() => {
    setBaseUrl(connection?.base_url ?? "");
  }, [connection?.base_url]);

  const connectionPayload = useMemo(
    () => ({
      provider,
      name: connection?.name || copy.title,
      base_url: baseUrl.trim() || null,
      status: connection?.status || "active",
      sync_enabled: connection?.sync_enabled === true,
      config: {
        sync_scopes: syncScopes.map((scope) => scope.key),
        ...(connection?.config ?? {}),
      },
    }),
    [baseUrl, connection?.config, connection?.name, connection?.status, connection?.sync_enabled, copy.title, provider, syncScopes],
  );

  async function saveConnection() {
    if (!canManage || savingConnection) return;
    setSavingConnection(true);
    try {
      if (connection) {
        await api.updateIntegrationConnection(wsId, connection.id, connectionPayload);
      } else {
        await api.createIntegrationConnection(wsId, connectionPayload);
      }
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
      toast.success(t(($) => $.enterprise_integrations.connection_saved));
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.enterprise_integrations.connection_save_failed),
      );
    } finally {
      setSavingConnection(false);
    }
  }

  if (!definition) return null;

  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
              {definition.icon}
            </div>
            <div className="min-w-0 space-y-1">
              <h3 className="text-body font-semibold">{copy.title}</h3>
              <p className="text-caption text-muted-foreground">{copy.subtitle}</p>
            </div>
          </div>
          <Button
            type="button"
            size="sm"
            onClick={saveConnection}
            disabled={!canManage || savingConnection}
            className="shrink-0 md:self-start"
          >
            <Save className="h-3 w-3" />
            {savingConnection
              ? t(($) => $.enterprise_integrations.saving)
              : t(($) => $.enterprise_integrations.save_connection)}
          </Button>
        </div>

        <div className="space-y-2">
          <Label htmlFor={`${provider}-base-url`}>
            {t(($) => $.enterprise_integrations.workspace_connection)}
          </Label>
          <Input
            id={`${provider}-base-url`}
            value={baseUrl}
            onChange={(event) => setBaseUrl(event.target.value)}
            placeholder={copy.baseUrlPlaceholder}
            disabled={!canManage}
          />
          {!canManage && (
            <p className="text-caption text-muted-foreground">
              {t(($) => $.enterprise_integrations.admin_required)}
            </p>
          )}
        </div>

        <div className="flex flex-col gap-1 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="text-caption font-medium text-muted-foreground">
            {t(($) => $.enterprise_integrations.my_account)}
          </div>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.enterprise_integrations.account_management_hint)}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

function IntegrationCenterOverview() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const user = useAuthStore((s) => s.user);
  const navigation = useNavigation();
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data, isLoading } = useQuery(integrationsOptions(wsId));
  const connections = data?.connections ?? [];
  const accounts = data?.accounts ?? [];
  const bindings = data?.project_bindings ?? [];
  const syncEvents = data?.sync_events ?? [];
  const providerCopy = useProviderCopy();
  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManage =
    currentMember?.role === "owner" || currentMember?.role === "admin";

  const providerTargets: Record<IntegrationProvider, string> = {
    gitlab: "github",
    zentao: "zentao",
    feishu: "feishu",
  };

  return (
    <div className="space-y-8">
      <section className="space-y-1">
        <h2 className="text-body font-semibold">
          {t(($) => $.enterprise_integrations.center_title)}
        </h2>
        <p className="text-body text-muted-foreground">
          {t(($) => $.enterprise_integrations.center_description)}
        </p>
      </section>

      {isLoading ? (
        <Card>
          <CardContent>
            <p className="text-body text-muted-foreground">{t(($) => $.lark.loading)}</p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-3">
          {PROVIDERS.map((definition) => {
            const connection =
              connections.find((item) => item.provider === definition.provider) ?? null;
            const accountCount = connection
              ? accounts.filter((account) => account.connection_id === connection.id).length
              : 0;
            const bindingCount = connection
              ? bindings.filter((binding) => binding.connection_id === connection.id).length
              : 0;
            const latestEvent =
              syncEvents.find((event) => event.provider === definition.provider) ?? null;
            // Surface a degraded account (e.g. expired/invalid token flagged by the
            // inbound worker) so the card doesn't read "active" when sync is broken.
            const needsAttention =
              connection != null &&
              accounts.some(
                (account) =>
                  account.connection_id === connection.id &&
                  (account.status === "error" || !!account.last_error),
              );
            return (
              <Card key={definition.provider}>
                <CardContent className="space-y-4">
                  <div className="flex items-start gap-3">
                    <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
                      {definition.icon}
                    </div>
                    <div className="min-w-0 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="text-body font-semibold">
                          {providerCopy[definition.provider].title}
                        </h3>
                        <span className="rounded bg-muted px-1.5 py-0.5 text-micro text-muted-foreground">
                          {connection?.sync_enabled
                            ? t(($) => $.enterprise_integrations.sync_on)
                            : t(($) => $.enterprise_integrations.sync_off)}
                        </span>
                        {needsAttention && (
                          <span className="rounded bg-amber-500/15 px-1.5 py-0.5 text-micro text-amber-700">
                            {t(($) => $.integrations.status.action_needed)}
                          </span>
                        )}
                      </div>
                      <p className="text-caption text-muted-foreground">
                        {connection?.base_url || t(($) => $.enterprise_integrations.not_connected)}
                      </p>
                    </div>
                  </div>
                  <div className="grid grid-cols-2 gap-2 text-caption text-muted-foreground">
                    <div>
                      <div className="font-medium text-foreground">{accountCount}</div>
                      {t(($) => $.enterprise_integrations.accounts_count)}
                    </div>
                    <div>
                      <div className="font-medium text-foreground">{bindingCount}</div>
                      {t(($) => $.enterprise_integrations.bindings_count)}
                    </div>
                  </div>
                  <EventHealthLine event={latestEvent} />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      navigation.push(`${navigation.pathname}?tab=${providerTargets[definition.provider]}`)
                    }
                  >
                    <ExternalLink className="h-3 w-3" />
                    {t(($) => $.enterprise_integrations.open_settings)}
                  </Button>
                </CardContent>
              </Card>
            );
          })}
        </div>
      )}

      {!isLoading && (
        <>
          <WorkspaceAccountsOverview
            accounts={accounts}
            connections={connections}
            canManage={canManage}
          />
          <LarkTab />
          <RecentSyncEvents events={syncEvents.slice(0, 6)} />
        </>
      )}
    </div>
  );
}

function WorkspaceAccountsOverview({
  accounts,
  connections,
  canManage,
}: {
  accounts: IntegrationUserAccount[];
  connections: IntegrationConnection[];
  canManage: boolean;
}) {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const [deletingId, setDeletingId] = useState<string | null>(null);

  async function deleteAccount(account: IntegrationUserAccount) {
    if (!canManage || deletingId) return;
    setDeletingId(account.id);
    try {
      await api.deleteIntegrationUserAccountById(wsId, account.id);
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
      toast.success(t(($) => $.tokens.external_accounts_deleted));
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t(($) => $.tokens.external_accounts_delete_failed),
      );
    } finally {
      setDeletingId(null);
    }
  }

  const connectionById = new Map(connections.map((connection) => [connection.id, connection]));

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="space-y-1">
          <h3 className="text-body font-semibold">
            {t(($) => $.enterprise_integrations.workspace_accounts_title)}
          </h3>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.enterprise_integrations.workspace_accounts_description)}
          </p>
        </div>
        {accounts.length === 0 ? (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.enterprise_integrations.workspace_accounts_empty)}
          </p>
        ) : (
          <div className="divide-y rounded-md border">
            {accounts.map((account) => {
              const connection = connectionById.get(account.connection_id);
              return (
                <div
                  key={account.id}
                  className="flex items-start justify-between gap-3 px-3 py-2"
                >
                  <div className="min-w-0 space-y-1">
                    <div className="flex flex-wrap items-center gap-2 text-caption">
                      <span className="font-medium text-foreground">
                        {account.account_name}
                      </span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-micro text-muted-foreground">
                        {connection?.name ?? account.connection_id}
                      </span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-micro text-muted-foreground">
                        {account.status}
                      </span>
                    </div>
                    <p className="truncate text-caption text-muted-foreground">
                      {account.external_username || account.external_user_id || account.account_key}
                    </p>
                    {account.scopes.length > 0 && (
                      <p className="truncate text-micro text-muted-foreground">
                        {account.scopes.join(", ")}
                      </p>
                    )}
                  </div>
                  {canManage && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      disabled={deletingId === account.id}
                      aria-label={t(($) => $.tokens.external_accounts_delete_aria, {
                        name: account.account_name,
                      })}
                      onClick={() => {
                        void deleteAccount(account);
                      }}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function EventHealthLine({ event }: { event: IntegrationSyncEvent | null }) {
  const { t } = useT("settings");
  const statusLabels = {
    success: t(($) => $.enterprise_integrations.event_status_success),
    warning: t(($) => $.enterprise_integrations.event_status_warning),
    error: t(($) => $.enterprise_integrations.event_status_error),
    skipped: t(($) => $.enterprise_integrations.event_status_skipped),
  };
  if (!event) {
    return (
      <p className="text-caption text-muted-foreground">
        {t(($) => $.enterprise_integrations.no_sync_events)}
      </p>
    );
  }
  return (
    <div className="rounded-md border bg-muted/30 px-3 py-2 text-caption">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium text-foreground">
          {statusLabels[event.status as keyof typeof statusLabels] ?? event.status}
        </span>
        <span className="text-muted-foreground">
          {formatIntegrationEventTime(event.occurred_at)}
        </span>
      </div>
      <p className="mt-1 line-clamp-2 text-muted-foreground">
        {event.error || event.message || event.object_type}
      </p>
    </div>
  );
}

function RecentSyncEvents({ events }: { events: IntegrationSyncEvent[] }) {
  const { t } = useT("settings");
  const statusLabels = {
    success: t(($) => $.enterprise_integrations.event_status_success),
    warning: t(($) => $.enterprise_integrations.event_status_warning),
    error: t(($) => $.enterprise_integrations.event_status_error),
    skipped: t(($) => $.enterprise_integrations.event_status_skipped),
  };
  const directionLabels = {
    inbound: t(($) => $.enterprise_integrations.event_direction_inbound),
    outbound: t(($) => $.enterprise_integrations.event_direction_outbound),
    internal: t(($) => $.enterprise_integrations.event_direction_internal),
  };
  return (
    <Card>
      <CardContent className="space-y-3">
        <h3 className="text-body font-semibold">
          {t(($) => $.enterprise_integrations.recent_events_title)}
        </h3>
        {events.length === 0 ? (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.enterprise_integrations.no_sync_events)}
          </p>
        ) : (
          <div className="divide-y rounded-md border">
            {events.map((event) => (
              <div key={event.id} className="flex items-start justify-between gap-4 px-3 py-2">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2 text-caption">
                    <span className="font-medium text-foreground">{event.provider}</span>
                    <span className="rounded bg-muted px-1.5 py-0.5 text-micro text-muted-foreground">
                      {directionLabels[event.direction as keyof typeof directionLabels] ?? event.direction}
                    </span>
                    <span className="rounded bg-muted px-1.5 py-0.5 text-micro text-muted-foreground">
                      {statusLabels[event.status as keyof typeof statusLabels] ?? event.status}
                    </span>
                  </div>
                  <p className="mt-1 truncate text-caption text-muted-foreground">
                    {event.error || event.message || event.object_type}
                  </p>
                </div>
                <span className="shrink-0 text-caption text-muted-foreground">
                  {formatIntegrationEventTime(event.occurred_at)}
                </span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function useProviderCopy(): Record<IntegrationProvider, ProviderCopy> {
  const { t } = useT("settings");
  return {
    gitlab: {
      title: t(($) => $.enterprise_integrations.providers.gitlab.title),
      subtitle: t(($) => $.enterprise_integrations.providers.gitlab.subtitle),
      baseUrlPlaceholder: t(
        ($) => $.enterprise_integrations.providers.gitlab.base_url_placeholder,
      ),
      accountPlaceholder: t(
        ($) => $.enterprise_integrations.providers.gitlab.account_placeholder,
      ),
      credentialPlaceholder: t(
        ($) => $.enterprise_integrations.providers.gitlab.credential_placeholder,
      ),
      connectionHelp: t(
        ($) => $.enterprise_integrations.providers.gitlab.connection_help,
      ),
      accountHelp: t(
        ($) => $.enterprise_integrations.providers.gitlab.account_help,
      ),
      credentialHelp: t(
        ($) => $.enterprise_integrations.providers.gitlab.credential_help,
      ),
    },
    zentao: {
      title: t(($) => $.enterprise_integrations.providers.zentao.title),
      subtitle: t(($) => $.enterprise_integrations.providers.zentao.subtitle),
      baseUrlPlaceholder: t(
        ($) => $.enterprise_integrations.providers.zentao.base_url_placeholder,
      ),
      accountPlaceholder: t(
        ($) => $.enterprise_integrations.providers.zentao.account_placeholder,
      ),
      credentialPlaceholder: t(
        ($) => $.enterprise_integrations.providers.zentao.credential_placeholder,
      ),
      connectionHelp: t(
        ($) => $.enterprise_integrations.providers.zentao.connection_help,
      ),
      accountHelp: t(
        ($) => $.enterprise_integrations.providers.zentao.account_help,
      ),
      credentialHelp: t(
        ($) => $.enterprise_integrations.providers.zentao.credential_help,
      ),
    },
    feishu: {
      title: t(($) => $.enterprise_integrations.providers.feishu.title),
      subtitle: t(($) => $.enterprise_integrations.providers.feishu.subtitle),
      baseUrlPlaceholder: t(
        ($) => $.enterprise_integrations.providers.feishu.base_url_placeholder,
      ),
      accountPlaceholder: t(
        ($) => $.enterprise_integrations.providers.feishu.account_placeholder,
      ),
      credentialPlaceholder: t(
        ($) => $.enterprise_integrations.providers.feishu.credential_placeholder,
      ),
      connectionHelp: t(
        ($) => $.enterprise_integrations.providers.feishu.connection_help,
      ),
      accountHelp: t(
        ($) => $.enterprise_integrations.providers.feishu.account_help,
      ),
      credentialHelp: t(
        ($) => $.enterprise_integrations.providers.feishu.credential_help,
      ),
    },
  };
}

function ProviderCard({
  definition,
  copy,
  connection,
  canManage,
}: {
  definition: ProviderDefinition;
  copy: ProviderCopy;
  connection: IntegrationConnection | null;
  canManage: boolean;
}) {
  const wsId = useWorkspaceId();
  const { t } = useT("settings");
  const qc = useQueryClient();
  const [baseUrl, setBaseUrl] = useState("");
  const [workspaceSync, setWorkspaceSync] = useState(false);
  const syncScopes = useProviderSyncScopes(definition.provider);
  const [enabledScopes, setEnabledScopes] = useState<Set<string>>(
    () => new Set(syncScopes.map((scope) => scope.key)),
  );
  const [savingConnection, setSavingConnection] = useState(false);
  // Feishu/Lark app credentials live at the connection (admin) level. app_id is
  // plaintext; app_secret is write-once (masked on read as app_secret_configured).
  const isFeishu = definition.provider === "feishu";
  const [appId, setAppId] = useState("");
  const [appSecret, setAppSecret] = useState("");
  const appSecretConfigured = connection?.config?.app_secret_configured === true;

  useEffect(() => {
    setBaseUrl(connection?.base_url ?? "");
    setWorkspaceSync(connection?.sync_enabled === true);
    setAppId(String(connection?.config?.app_id ?? ""));
    setAppSecret("");
    const savedScopes = connection?.config?.sync_scopes;
    setEnabledScopes(
      new Set(
        Array.isArray(savedScopes) && savedScopes.length > 0
          ? savedScopes.filter((scope): scope is string => typeof scope === "string")
          : syncScopes.map((scope) => scope.key),
      ),
    );
  }, [connection?.base_url, connection?.config, connection?.sync_enabled, syncScopes]);

  const connectionPayload = useMemo(
    () => ({
      provider: definition.provider,
      name: copy.title,
      base_url: baseUrl.trim() || null,
      status: "active",
      sync_enabled: workspaceSync,
      config: {
        ...(connection?.config ?? {}),
        sync_scopes: syncScopes
          .map((scope) => scope.key)
          .filter((key) => enabledScopes.has(key)),
        ...(isFeishu
          ? {
              app_id: appId.trim(),
              // Only send app_secret when the admin (re)entered it; the backend
              // keeps the existing encrypted value when this is omitted.
              ...(appSecret.trim() ? { app_secret: appSecret.trim() } : {}),
            }
          : {}),
      },
    }),
    [appId, appSecret, baseUrl, connection?.config, copy.title, definition.provider, enabledScopes, isFeishu, syncScopes, workspaceSync],
  );

  function toggleScope(key: string, enabled: boolean) {
    setEnabledScopes((current) => {
      const next = new Set(current);
      if (enabled) next.add(key);
      else next.delete(key);
      return next;
    });
  }

  async function saveConnection() {
    if (!canManage || savingConnection) return;
    setSavingConnection(true);
    try {
      if (connection) {
        await api.updateIntegrationConnection(wsId, connection.id, connectionPayload);
      } else {
        await api.createIntegrationConnection(wsId, connectionPayload);
      }
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
      toast.success(t(($) => $.enterprise_integrations.connection_saved));
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.enterprise_integrations.connection_save_failed),
      );
    } finally {
      setSavingConnection(false);
    }
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
              {definition.icon}
            </div>
            <div className="min-w-0 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-body font-semibold">{copy.title}</h3>
                <span className="rounded bg-muted px-1.5 py-0.5 text-micro text-muted-foreground">
                  {connection?.sync_enabled === true
                    ? t(($) => $.enterprise_integrations.sync_on)
                    : t(($) => $.enterprise_integrations.sync_off)}
                </span>
              </div>
              <p className="text-caption text-muted-foreground">{copy.subtitle}</p>
            </div>
          </div>
          <Button
            type="button"
            size="sm"
            onClick={saveConnection}
            disabled={!canManage || savingConnection}
            className="shrink-0 md:self-start"
          >
            <Save className="h-3 w-3" />
            {savingConnection
              ? t(($) => $.enterprise_integrations.saving)
              : t(($) => $.enterprise_integrations.save_connection)}
          </Button>
        </div>

        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_240px] md:items-end">
          <div className="space-y-2">
            <Label htmlFor={`${definition.provider}-base-url`}>
              {t(($) => $.enterprise_integrations.workspace_connection)}
            </Label>
            <Input
              id={`${definition.provider}-base-url`}
              value={baseUrl}
              onChange={(event) => setBaseUrl(event.target.value)}
              placeholder={copy.baseUrlPlaceholder}
              disabled={!canManage}
            />
          </div>
          <ToggleRow
            id={`${definition.provider}-workspace-sync`}
            label={t(($) => $.enterprise_integrations.enable_workspace_sync)}
            checked={workspaceSync}
            onCheckedChange={setWorkspaceSync}
            disabled={!canManage}
          />
          {!canManage && (
            <p className="text-caption text-muted-foreground">
              {t(($) => $.enterprise_integrations.admin_required)}
            </p>
          )}
        </div>

        {isFeishu && (
          <div className="grid gap-3 border-t pt-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor={`${definition.provider}-app-id`}>
                {t(($) => $.enterprise_integrations.feishu_app_id_label)}
              </Label>
              <Input
                id={`${definition.provider}-app-id`}
                value={appId}
                onChange={(event) => setAppId(event.target.value)}
                placeholder={t(($) => $.enterprise_integrations.feishu_app_id_placeholder)}
                disabled={!canManage}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${definition.provider}-app-secret`}>
                {t(($) => $.enterprise_integrations.feishu_app_secret_label)}
              </Label>
              <Input
                id={`${definition.provider}-app-secret`}
                type="password"
                value={appSecret}
                onChange={(event) => setAppSecret(event.target.value)}
                placeholder={
                  appSecretConfigured
                    ? t(($) => $.enterprise_integrations.feishu_app_secret_configured)
                    : t(($) => $.enterprise_integrations.feishu_app_secret_placeholder)
                }
                disabled={!canManage}
              />
            </div>
          </div>
        )}

        <div className="space-y-2 border-t pt-4">
          <div className="text-caption font-medium text-muted-foreground">
            {t(($) => $.enterprise_integrations.sync_scope_title)}
          </div>
          <div className="grid gap-2 md:grid-cols-2">
            {syncScopes.map((scope) => (
              <ToggleRow
                key={scope.key}
                id={`${definition.provider}-scope-${scope.key}`}
                label={scope.label}
                checked={enabledScopes.has(scope.key)}
                onCheckedChange={(value) => toggleScope(scope.key, value)}
                disabled={!canManage}
              />
            ))}
          </div>
        </div>

        <div className="flex flex-col gap-1 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="text-caption font-medium text-muted-foreground">
            {t(($) => $.enterprise_integrations.my_account)}
          </div>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.enterprise_integrations.account_management_hint)}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

function useProviderSyncScopes(provider: IntegrationProvider): { key: string; label: string }[] {
  const { t } = useT("settings");
  return useMemo(() => {
    if (provider === "gitlab") {
      return [
        { key: "gitlab_issues", label: t(($) => $.enterprise_integrations.sync_scopes.gitlab_issues) },
        { key: "gitlab_merge_requests", label: t(($) => $.enterprise_integrations.sync_scopes.gitlab_merge_requests) },
        { key: "gitlab_repository_context", label: t(($) => $.enterprise_integrations.sync_scopes.gitlab_repository_context) },
      ];
    }
    if (provider === "zentao") {
      return [
        { key: "zentao_bugs", label: t(($) => $.enterprise_integrations.sync_scopes.zentao_bugs) },
        { key: "zentao_stories", label: t(($) => $.enterprise_integrations.sync_scopes.zentao_stories) },
        { key: "zentao_tasks", label: t(($) => $.enterprise_integrations.sync_scopes.zentao_tasks) },
        { key: "zentao_comments", label: t(($) => $.enterprise_integrations.sync_scopes.zentao_comments) },
        { key: "zentao_outbound", label: t(($) => $.enterprise_integrations.sync_scopes.zentao_outbound) },
      ];
    }
    return [
      { key: "feishu_bot_im", label: t(($) => $.enterprise_integrations.sync_scopes.feishu_bot_im) },
      { key: "feishu_drive", label: t(($) => $.enterprise_integrations.sync_scopes.feishu_drive) },
      { key: "feishu_wiki", label: t(($) => $.enterprise_integrations.sync_scopes.feishu_wiki) },
      { key: "feishu_docs", label: t(($) => $.enterprise_integrations.sync_scopes.feishu_docs) },
    ];
  }, [provider, t]);
}

function ToggleRow({
  id,
  label,
  checked,
  disabled,
  onCheckedChange,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled?: boolean;
  onCheckedChange: (value: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <Label htmlFor={id} className="text-caption font-medium text-muted-foreground">
        {label}
      </Label>
      <Switch
        id={id}
        size="sm"
        checked={checked}
        disabled={disabled}
        onCheckedChange={onCheckedChange}
      />
    </div>
  );
}

function formatIntegrationEventTime(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
