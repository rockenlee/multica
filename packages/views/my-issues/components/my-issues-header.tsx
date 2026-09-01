"use client";

import { useEffect } from "react";
import { useStore } from "zustand";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, Settings2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { integrationsOptions, integrationKeys } from "@multica/core/integrations";
import { useWorkspaceId } from "@multica/core/hooks";
import { useEffect, useState } from "react";
import { ChevronDown } from "lucide-react";
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
import type {
  Issue,
  IssueTableFacetSpec,
  IssueTableFacetsResponse,
  WorkingAgentSummary,
} from "@multica/core/types";
import { myIssuesViewStore, type MyIssuesScope } from "@multica/core/issues/stores/my-issues-view-store";
import {
  ISSUE_SYNC_PROVIDERS,
  normalizeIssueSourceProvider,
  type IssueSyncProvider,
  type IssueSyncSettings,
} from "@multica/core/issues";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import { useT } from "../../i18n";
import { WorkspaceAgentWorkingChip } from "../../issues/components/workspace-agent-working-chip";
import {
  IssueDisplayControls,
  ViewRefreshIndicator,
} from "../../issues/components/issues-header";
import { cn } from "@multica/ui/lib/utils";
import { PAGE_GUTTER } from "../../layout/page-header";
import { FilterChipsBar } from "../../issues/components/filter-chips-bar";
import { toast } from "sonner";
import { SaveViewDialog, type SaveViewScope } from "../../issues/components/save-view-dialog";
import { useActiveIssueView } from "@multica/core/issue-views/use-active-view";
import { useWorkspaceId } from "@multica/core/hooks";
import { baselineFromQuery } from "@multica/core/issue-views/baseline";
import { ViewBar } from "../../issues/components/view-bar";
import type { IssueView } from "@multica/core/api/schemas";

/** My Issues tab → saved-view scope_variant (API vocabulary). */
const SAVE_VARIANT: Record<MyIssuesScope, Extract<SaveViewScope, { kind: "my" }>["variant"]> = {
  all: "any",
  assigned: "assigned",
  created: "created",
  agents: "involved",
};

const SOURCE_LABEL_KEY: Record<
  IssueSyncProvider,
  "source_feishu" | "source_zentao" | "source_gitlab"
> = {
  feishu: "source_feishu",
  zentao: "source_zentao",
  gitlab: "source_gitlab",
};

export function MyIssuesHeader({
  allIssues,
  workingAgents,
  scope,
  onScopeChange,
  isRefreshing = false,
  facetCountsExact = true,
  tableFacetCounts,
  onTableFacetChange,
}: {
  allIssues: Issue[];
  /** See IssueSurfaceController.workingAgents. My Issues used to ask the
   *  working-agents endpoint for its own relation-scoped count; the surface
   *  projection now covers the relation AND every active filter. */
  workingAgents: WorkingAgentSummary[] | undefined;
  scope: MyIssuesScope;
  onScopeChange: (scope: MyIssuesScope) => void;
  isRefreshing?: boolean;
  /** See IssueDisplayControls.facetCountsExact. */
  facetCountsExact?: boolean;
  tableFacetCounts?: IssueTableFacetsResponse;
  onTableFacetChange: (facet: IssueTableFacetSpec | null) => void;
}) {
  const { t } = useT("my-issues");
  const { t: tIssues } = useT("issues");
  const [saveViewOpen, setSaveViewOpen] = useState(false);
  const saveScope: SaveViewScope = { kind: "my", variant: SAVE_VARIANT[scope] };
  const wsId = useWorkspaceId();
  const { activeView, views, viewsReady, setActive, missing } = useActiveIssueView(
    wsId,
    { scope_type: "my" },
  );
  useEffect(() => {
    if (missing) {
      setActive(null);
      toast.info(tIssues(($) => $.view_selector.unavailable));
    }
  }, [missing, setActive, tIssues]);
  const viewBaseline = activeView ? baselineFromQuery(activeView.query) : undefined;
  const [editTarget, setEditTarget] = useState<{
    view: IssueView;
    fromDefinition: boolean;
  } | null>(null);
  const SCOPES: { value: MyIssuesScope; label: string; description: string }[] = [
    { value: "all", label: t(($) => $.header.scope.all_label), description: t(($) => $.header.scope.all_description) },
    { value: "assigned", label: t(($) => $.header.scope.assigned_label), description: t(($) => $.header.scope.assigned_description) },
    { value: "created", label: t(($) => $.header.scope.created_label), description: t(($) => $.header.scope.created_description) },
    { value: "agents", label: t(($) => $.header.scope.agents_label), description: t(($) => $.header.scope.agents_description) },
  ];
  const agentRunningFilter = useViewStore((s) => s.agentRunningFilter);
  const issueSourceFilters = useStore(myIssuesViewStore, (s) => s.sourceFilters);
  const toggleAgentRunningFilter = useViewStore(
    (s) => s.toggleAgentRunningFilter,
  );
  const toggleIssueSourceFilter = useStore(
    myIssuesViewStore,
    (s) => s.toggleSourceFilter,
  );
  const scopeLabel = SCOPES.find((s) => s.value === scope)?.label ?? SCOPES[0]?.label;

  return (
    <>
    <div className={cn("min-h-12 shrink-0 py-2 [-webkit-overflow-scrolling:touch]", PAGE_GUTTER)}>
      <div className="flex w-full min-w-0 items-start justify-between gap-2">
        <div className="hidden min-w-0 flex-1 md:block">
          <ViewBar
            wsId={wsId}
            scope={{ scope_type: "my" }}
            builtins={SCOPES.map((s) => ({
              key: s.value,
              label: s.label,
              description: s.description,
              active: !activeView && scope === s.value,
              onSelect: () => {
                if (activeView) setActive(null);
                onScopeChange(s.value);
              },
            }))}
            views={views}
            viewsReady={viewsReady}
            activeView={activeView}
            onSelectView={(view) => setActive(view ? view.id : null)}
            onNewView={() => {
              setEditTarget(null);
              setSaveViewOpen(true);
            }}
            onEditView={(view) => {
              setEditTarget({ view, fromDefinition: true });
              setSaveViewOpen(true);
            }}
          />
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
              onValueChange={(value) => onScopeChange(value as MyIssuesScope)}
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
            <span className="mr-1 hidden text-caption text-muted-foreground md:inline">
              {tIssues(($) => $.agent_activity.filter_active_label)}
            </span>
          )}
          <WorkspaceAgentWorkingChip
            value={agentRunningFilter}
            onToggle={toggleAgentRunningFilter}
            agents={workingAgents}
          />
          <MyIssuesSyncSettingsButton />
          <IssueDisplayControls
            scopedIssues={allIssues}
            issueSourceFilters={issueSourceFilters}
            onToggleIssueSourceFilter={toggleIssueSourceFilter}
            onClearIssueSourceFilters={() =>
              myIssuesViewStore.setState({ sourceFilters: [] })
            }
            facetCountsExact={facetCountsExact}
            tableFacetCounts={tableFacetCounts}
            onTableFacetChange={onTableFacetChange}
            viewBaseline={viewBaseline}
          />
          <ViewRefreshIndicator active={isRefreshing} />
        </div>
      </div>
    </div>
    <FilterChipsBar
      viewBaseline={viewBaseline}
      saveLabel={activeView ? tIssues(($) => $.filters.chip_edit) : undefined}
      onSave={() => {
        setEditTarget(
          activeView ? { view: activeView, fromDefinition: false } : null,
        );
        setSaveViewOpen(true);
      }}
    />
    <SaveViewDialog
      open={saveViewOpen}
      onOpenChange={setSaveViewOpen}
      scope={saveScope}
      editView={editTarget?.view ?? null}
      seedFromDefinition={editTarget?.fromDefinition ?? false}
    />
    </>
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
            <h2 className="text-body font-medium">{t(($) => $.sync_settings.title)}</h2>
            <p className="mt-1 text-caption leading-5 text-muted-foreground">
              {t(($) => $.sync_settings.description)}
            </p>
          </div>

          <div className="rounded-md border">
            <div className="grid grid-cols-[1fr_72px_72px] gap-2 border-b px-3 py-2 text-micro font-medium uppercase text-muted-foreground">
              <span>{t(($) => $.sync_settings.channel_column)}</span>
              <span className="text-center">{t(($) => $.sync_settings.inbound_column)}</span>
              <span className="text-center">{t(($) => $.sync_settings.outbound_column)}</span>
            </div>
            {ISSUE_SYNC_PROVIDERS.map((provider) => (
              <div
                key={provider}
                className="grid grid-cols-[1fr_72px_72px] items-center gap-2 border-b px-3 py-2 last:border-b-0"
              >
                <span className="text-body">{tIssues(($) => $.sync[SOURCE_LABEL_KEY[provider]])}</span>
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

          <p className="text-caption leading-5 text-muted-foreground">
            {t(($) => $.sync_settings.resource_hint)}
          </p>
        </div>
      </PopoverContent>
    </Popover>
  );
}
