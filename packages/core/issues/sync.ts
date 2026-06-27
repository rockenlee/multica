import type { Issue } from "../types";

export const ISSUE_SYNC_PROVIDERS = ["feishu", "zentao", "gitlab"] as const;

export type IssueSyncProvider = (typeof ISSUE_SYNC_PROVIDERS)[number];

export interface IssueSyncChannelSettings {
  inbound: boolean;
  outbound: boolean;
}

export type IssueSyncSettings = Record<IssueSyncProvider, IssueSyncChannelSettings>;

export const DEFAULT_ISSUE_SYNC_SETTINGS: IssueSyncSettings = {
  feishu: { inbound: false, outbound: false },
  zentao: { inbound: false, outbound: false },
  gitlab: { inbound: false, outbound: false },
};

export function normalizeIssueSourceProvider(value: unknown): IssueSyncProvider | null {
  if (typeof value !== "string") return null;
  const normalized = value.trim().toLowerCase();
  if (normalized === "lark" || normalized === "feishu") return "feishu";
  if (normalized === "zentao") return "zentao";
  if (normalized === "gitlab") return "gitlab";
  return null;
}

export function getIssueSourceProvider(issue: Pick<Issue, "metadata">): IssueSyncProvider | null {
  return normalizeIssueSourceProvider(issue.metadata?.source_system);
}
