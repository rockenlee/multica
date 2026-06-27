"use client";

import { useEffect, useMemo } from "react";
import { useStore } from "zustand";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Settings2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { integrationsOptions, integrationKeys } from "@multica/core/integrations";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { Switch } from "@multica/ui/components/ui/switch";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";
import type { Issue } from "@multica/core/types";
import { myIssuesViewStore, type MyIssuesScope } from "@multica/core/issues/stores/my-issues-view-store";
import {
  ISSUE_SYNC_PROVIDERS,
  normalizeIssueSourceProvider,
  type IssueSyncProvider,
  type IssueSyncSettings,
} from "@multica/core/issues";
import { useT } from "../../i18n";
import { WorkspaceAgentWorkingChip } from "../../issues/components/workspace-agent-working-chip";
import { IssueDisplayControls } from "../../issues/components/issues-header";

const SOURCE_LABEL_KEY: Record<IssueSyncProvider, "source_feishu" | "source_zentao" | "source_gitlab"> = {
  feishu: "source_feishu",
  zentao: "source_zentao",
  gitlab: "source_gitlab",
};

export function MyIssuesHeader({ allIssues }: { allIssues: Issue[] }) {
  const { t } = useT("my-issues");
  const { t: tIssues } = useT("issues");
  const SCOPES: { value: MyIssuesScope; label: string; description: string }[] = [
    { value: "all", label: t(($) => $.header.scope.all_label), description: t(($) => $.header.scope.all_description) },
    { value: "assigned", label: t(($) => $.header.scope.assigned_label), description: t(($) => $.header.scope.assigned_description) },
    { value: "created", label: t(($) => $.header.scope.created_label), description: t(($) => $.header.scope.created_description) },
    { value: "agents", label: t(($) => $.header.scope.agents_label), description: t(($) => $.header.scope.agents_description) },
  ];
  const scope = useStore(myIssuesViewStore, (s) => s.scope);
  const agentRunningFilter = useStore(myIssuesViewStore, (s) => s.agentRunningFilter);
  const sourceFilters = useStore(myIssuesViewStore, (s) => s.sourceFilters);
  const act = myIssuesViewStore.getState();
  const scopedIssueIds = useMemo(
    () => new Set(allIssues.map((i) => i.id)),
    [allIssues],
  );
  const scopeLabel = SCOPES.find((s) => s.value === scope)?.label ?? SCOPES[0]?.label;

  return (
    <div className="h-12 shrink-0 overflow-x-auto px-4 [-webkit-overflow-scrolling:touch]">
      <div className="flex h-full w-max min-w-full items-center justify-between gap-2">
        <div className="hidden shrink-0 items-center gap-1 md:flex">
          {SCOPES.map((s) => (
            <Tooltip key={s.value}>
              <TooltipTrigger
                render={
                  <Button
                    variant="outline"
                    size="sm"
                    className={
                      scope === s.value
                        ? "bg-accent text-accent-foreground hover:bg-accent/80"
                        : "text-muted-foreground"
                    }
                    onClick={() => act.setScope(s.value)}
                  >
                    {s.label}
                  </Button>
                }
              />
              <TooltipContent side="bottom">{s.description}</TooltipContent>
            </Tooltip>
          ))}
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="outline"
                size="sm"
                className="shrink-0 gap-1 text-muted-foreground md:hidden"
              >
                <span className="truncate">{scopeLabel}</span>
                <ChevronDown className="size-3 text-muted-foreground" />
              </Button>
            }
          />
          <DropdownMenuContent align="start" className="w-auto">
            <DropdownMenuRadioGroup
              value={scope}
              onValueChange={(value) => act.setScope(value as MyIssuesScope)}
            >
              {SCOPES.map((s) => (
                <DropdownMenuRadioItem key={s.value} value={s.value}>
                  {s.label}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>

        <div className="flex shrink-0 items-center gap-1">
          {agentRunningFilter && (
            <span className="mr-1 hidden text-xs text-muted-foreground md:inline">
              {tIssues(($) => $.agent_activity.filter_active_label)}
            </span>
          )}
          <WorkspaceAgentWorkingChip
            value={agentRunningFilter}
            onToggle={act.toggleAgentRunningFilter}
            scopedIssueIds={scopedIssueIds}
          />
          <IssueDisplayControls
            scopedIssues={allIssues}
            issueSourceFilters={sourceFilters}
            onToggleIssueSourceFilter={act.toggleSourceFilter}
            onClearIssueSourceFilters={() => myIssuesViewStore.setState({ sourceFilters: [] })}
          />
        </div>
      </div>
    </div>
  );
}

export function MyIssuesSyncSettingsButton() {
  const { t } = useT("my-issues");
  const { t: tIssues } = useT("issues");
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const { data } = useQuery(integrationsOptions(wsId));
  const syncSettings = useStore(myIssuesViewStore, (s) => s.syncSettings);
  const act = myIssuesViewStore.getState();

  useEffect(() => {
    if (!data?.issue_sync_settings) return;
    const next: Partial<IssueSyncSettings> = {};
    for (const setting of data.issue_sync_settings) {
      const provider = normalizeIssueSourceProvider(setting.provider);
      if (!provider) continue;
      next[provider] = {
        inbound: setting.inbound_enabled,
        outbound: setting.outbound_enabled,
      };
    }
    myIssuesViewStore.getState().setSyncSettings(next);
  }, [data?.issue_sync_settings]);

  const handleSyncChannelChange = async (
    provider: IssueSyncProvider,
    direction: keyof IssueSyncSettings[IssueSyncProvider],
    enabled: boolean,
  ) => {
    const before = myIssuesViewStore.getState().syncSettings;
    const current = before[provider];
    const nextChannel = { ...current, [direction]: enabled };
    act.setSyncChannel(provider, direction, enabled);
    try {
      await api.updateIntegrationIssueSyncSetting(wsId, provider, {
        inbound_enabled: nextChannel.inbound,
        outbound_enabled: nextChannel.outbound,
      });
      await queryClient.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
    } catch (err) {
      act.setSyncSettings(before);
      toast.error(err instanceof Error ? err.message : t(($) => $.sync_settings.save_failed));
    }
  };

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button variant="outline" size="sm" className="h-7 gap-1.5 text-muted-foreground">
            <Settings2 className="size-3.5" />
            <span className="hidden sm:inline">{t(($) => $.sync_settings.trigger)}</span>
          </Button>
        }
      />
      <PopoverContent align="start" className="w-[360px] p-3">
        <div className="space-y-3">
          <div>
            <h2 className="text-sm font-medium">{t(($) => $.sync_settings.title)}</h2>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {t(($) => $.sync_settings.description)}
            </p>
          </div>

          <div className="rounded-md border">
            <div className="grid grid-cols-[1fr_72px_72px] gap-2 border-b px-3 py-2 text-[11px] font-medium uppercase text-muted-foreground">
              <span>{t(($) => $.sync_settings.channel_column)}</span>
              <span className="text-center">{t(($) => $.sync_settings.inbound_column)}</span>
              <span className="text-center">{t(($) => $.sync_settings.outbound_column)}</span>
            </div>
            {ISSUE_SYNC_PROVIDERS.map((provider) => (
              <div
                key={provider}
                className="grid grid-cols-[1fr_72px_72px] items-center gap-2 border-b px-3 py-2 last:border-b-0"
              >
                <span className="text-sm">{tIssues(($) => $.sync[SOURCE_LABEL_KEY[provider]])}</span>
                <div className="flex justify-center">
                  <Switch
                    checked={syncSettings[provider]?.inbound ?? false}
                    onCheckedChange={(checked) => handleSyncChannelChange(provider, "inbound", checked)}
                    aria-label={t(($) => $.sync_settings.inbound_aria, {
                      provider: tIssues(($) => $.sync[SOURCE_LABEL_KEY[provider]]),
                    })}
                  />
                </div>
                <div className="flex justify-center">
                  <Switch
                    checked={syncSettings[provider]?.outbound ?? false}
                    onCheckedChange={(checked) => handleSyncChannelChange(provider, "outbound", checked)}
                    aria-label={t(($) => $.sync_settings.outbound_aria, {
                      provider: tIssues(($) => $.sync[SOURCE_LABEL_KEY[provider]]),
                    })}
                  />
                </div>
              </div>
            ))}
          </div>

          <p className="text-xs leading-5 text-muted-foreground">
            {t(($) => $.sync_settings.resource_hint)}
          </p>
        </div>
      </PopoverContent>
    </Popover>
  );
}
