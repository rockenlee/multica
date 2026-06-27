"use client";

import { createStore, type StoreApi } from "zustand/vanilla";
import { persist } from "zustand/middleware";
import {
  type IssueViewState,
  viewStoreSlice,
  viewStorePersistOptions,
  mergeViewStatePersisted,
} from "./view-store";
import { registerForWorkspaceRehydration } from "../../platform/workspace-storage";
import {
  DEFAULT_ISSUE_SYNC_SETTINGS,
  ISSUE_SYNC_PROVIDERS,
  type IssueSyncProvider,
  type IssueSyncSettings,
} from "../sync";

export type MyIssuesScope = "all" | "assigned" | "created" | "agents";

export interface MyIssuesViewState extends IssueViewState {
  scope: MyIssuesScope;
  sourceFilters: IssueSyncProvider[];
  syncSettings: IssueSyncSettings;
  setScope: (scope: MyIssuesScope) => void;
  toggleSourceFilter: (provider: IssueSyncProvider) => void;
  setSyncSettings: (settings: Partial<IssueSyncSettings>) => void;
  setSyncChannel: (
    provider: IssueSyncProvider,
    direction: keyof IssueSyncSettings[IssueSyncProvider],
    enabled: boolean,
  ) => void;
}

const basePersist = viewStorePersistOptions("multica_my_issues_view");

function mergeMyIssuesViewState(
  persisted: unknown,
  current: MyIssuesViewState,
): MyIssuesViewState {
  const merged = mergeViewStatePersisted<MyIssuesViewState>(persisted, current);
  const p = (persisted ?? {}) as Partial<MyIssuesViewState>;
  const syncSettings = { ...DEFAULT_ISSUE_SYNC_SETTINGS };
  for (const provider of ISSUE_SYNC_PROVIDERS) {
    syncSettings[provider] = {
      ...DEFAULT_ISSUE_SYNC_SETTINGS[provider],
      ...(p.syncSettings?.[provider] ?? {}),
    };
  }
  return {
    ...merged,
    sourceFilters: p.sourceFilters ?? current.sourceFilters,
    syncSettings,
  };
}

const _myIssuesViewStore = createStore<MyIssuesViewState>()(
  persist(
    (set) => ({
      ...viewStoreSlice(set as unknown as StoreApi<IssueViewState>["setState"]),
      scope: "assigned" as MyIssuesScope,
      sourceFilters: [],
      syncSettings: DEFAULT_ISSUE_SYNC_SETTINGS,
      setScope: (scope: MyIssuesScope) => set({ scope }),
      toggleSourceFilter: (provider: IssueSyncProvider) =>
        set((state) => ({
          sourceFilters: state.sourceFilters.includes(provider)
            ? state.sourceFilters.filter((p) => p !== provider)
            : [...state.sourceFilters, provider],
        })),
      setSyncSettings: (settings: Partial<IssueSyncSettings>) =>
        set((state) => {
          const next = { ...state.syncSettings };
          for (const provider of ISSUE_SYNC_PROVIDERS) {
            next[provider] = {
              ...DEFAULT_ISSUE_SYNC_SETTINGS[provider],
              ...next[provider],
              ...(settings[provider] ?? {}),
            };
          }
          return { syncSettings: next };
        }),
      setSyncChannel: (
        provider: IssueSyncProvider,
        direction: keyof IssueSyncSettings[IssueSyncProvider],
        enabled: boolean,
      ) =>
        set((state) => ({
          syncSettings: {
            ...state.syncSettings,
            [provider]: {
              ...DEFAULT_ISSUE_SYNC_SETTINGS[provider],
              ...state.syncSettings[provider],
              [direction]: enabled,
            },
          },
        })),
    }),
    {
      name: basePersist.name,
      storage: basePersist.storage,
      partialize: (state: MyIssuesViewState) => ({
        ...basePersist.partialize(state),
        scope: state.scope,
        sourceFilters: state.sourceFilters,
        syncSettings: state.syncSettings,
      }),
      // Reuse the same deep-merge as the base view store, then deep-merge
      // My Issues-only sync settings so newly added provider toggles inherit
      // defaults for existing users.
      merge: mergeMyIssuesViewState,
    },
  ),
);

export const myIssuesViewStore: StoreApi<MyIssuesViewState> = _myIssuesViewStore;

registerForWorkspaceRehydration(() => _myIssuesViewStore.persist.rehydrate());
