import type { Issue } from "./issue";

export type IntegrationProvider = "gitlab" | "zentao" | "feishu";

export interface IntegrationConnection {
  id: string;
  workspace_id: string;
  provider: IntegrationProvider | string;
  name: string;
  base_url: string | null;
  config: Record<string, unknown>;
  status: "active" | "disabled" | string;
  sync_enabled: boolean;
  created_by_user_id?: string | null;
  created_at: string;
  updated_at: string;
}

export interface IntegrationUserAccount {
  id: string;
  workspace_id: string;
  connection_id: string;
  user_id: string;
  account_key: string;
  account_name: string;
  external_user_id?: string | null;
  external_username?: string | null;
  credential_configured: boolean;
  scopes: string[];
  config: Record<string, unknown>;
  status: "active" | "disabled" | "error" | string;
  sync_enabled: boolean;
  expires_at?: string | null;
  last_used_at?: string | null;
  last_error?: string | null;
  created_at: string;
  updated_at: string;
}

export interface IntegrationProjectBinding {
  id: string;
  workspace_id: string;
  project_id: string;
  connection_id: string;
  external_ref: Record<string, unknown>;
  inbound_enabled: boolean;
  outbound_enabled: boolean;
  issue_sync_enabled: boolean;
  knowledge_sync_enabled: boolean;
  created_by_user_id?: string | null;
  created_at: string;
  updated_at: string;
}

export interface IntegrationSyncEvent {
  id: string;
  workspace_id: string;
  connection_id?: string | null;
  provider: IntegrationProvider | string;
  direction: "inbound" | "outbound" | "internal" | string;
  object_type: string;
  object_id?: string | null;
  external_id?: string | null;
  external_url?: string | null;
  project_id?: string | null;
  status: "success" | "warning" | "error" | "skipped" | string;
  message?: string | null;
  error?: string | null;
  metadata: Record<string, unknown>;
  occurred_at: string;
  created_at: string;
}

export interface IntegrationIssueSyncSetting {
  workspace_id: string;
  user_id: string;
  provider: IntegrationProvider | string;
  inbound_enabled: boolean;
  outbound_enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface RequestIssueOutboundSyncRequest {
  provider?: IntegrationProvider;
  source_type?: string;
}

export interface IssueOutboundSyncResponse {
  issue_id: string;
  project_id?: string | null;
  status: "queued" | "skipped";
  provider?: IntegrationProvider | null;
  connection_id?: string | null;
  message: string;
  metadata: Record<string, unknown>;
  event?: IntegrationSyncEvent;
}

export interface ListIntegrationsResponse {
  connections: IntegrationConnection[];
  accounts: IntegrationUserAccount[];
  project_bindings: IntegrationProjectBinding[];
  issue_sync_settings: IntegrationIssueSyncSetting[];
  sync_events: IntegrationSyncEvent[];
  credential_storage_enabled: boolean;
}

export interface UpsertIntegrationConnectionRequest {
  provider?: IntegrationProvider | string;
  name: string;
  base_url?: string | null;
  config?: Record<string, unknown>;
  status?: "active" | "disabled" | string;
  sync_enabled?: boolean;
}

export interface UpsertIntegrationUserAccountRequest {
  account_key?: string | null;
  account_name?: string | null;
  external_user_id?: string | null;
  external_username?: string | null;
  credential?: string;
  scopes?: string[];
  config?: Record<string, unknown>;
  status?: "active" | "disabled" | "error" | string;
  sync_enabled: boolean;
  expires_at?: string | null;
}

export interface UpsertIntegrationProjectBindingRequest {
  external_ref?: Record<string, unknown>;
  inbound_enabled: boolean;
  outbound_enabled: boolean;
  issue_sync_enabled: boolean;
  knowledge_sync_enabled: boolean;
}

export interface UpsertIntegrationIssueSyncSettingRequest {
  inbound_enabled: boolean;
  outbound_enabled: boolean;
}

export interface SyncInboundIntegrationIssueRequest {
  title: string;
  description?: string | null;
  source_type?: string | null;
  source_id: string;
  source_url?: string | null;
  external_status?: string | null;
  project_id?: string | null;
  resource_id?: string | null;
  assignee_user_id?: string | null;
}

export interface SyncInboundIntegrationIssueResponse {
  status: "created" | "updated" | string;
  issue: Issue;
  metadata: Record<string, unknown>;
  event: IntegrationSyncEvent;
}
