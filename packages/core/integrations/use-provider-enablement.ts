"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { deriveGitHubSettings } from "../github/settings";
import type { Workspace } from "../types";
import { integrationsOptions } from "./queries";

export interface ProviderEnablement {
  github: boolean;
  gitlab: boolean;
  feishu: boolean;
  zentao: boolean;
}

/**
 * Whether each external provider is turned on for the workspace. GitHub reads
 * the workspace settings master switch; GitLab / Feishu / ZenTao read their
 * integration connection's `sync_enabled` flag. Resource pickers (the create
 * dialog and the project resources panel) consult this so they only surface
 * providers the workspace has actually enabled.
 */
export function useProviderEnablement(
  wsId: string,
  workspace: Pick<Workspace, "settings"> | null | undefined,
): ProviderEnablement {
  const { data } = useQuery(integrationsOptions(wsId));
  return useMemo(() => {
    const connections = data?.connections ?? [];
    const syncEnabled = (provider: string) =>
      connections.some(
        (connection) => connection.provider === provider && connection.sync_enabled === true,
      );
    return {
      github: deriveGitHubSettings(workspace).enabled,
      gitlab: syncEnabled("gitlab"),
      feishu: syncEnabled("feishu"),
      zentao: syncEnabled("zentao"),
    };
  }, [data, workspace]);
}
