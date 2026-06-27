import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const integrationKeys = {
  all: (wsId: string) => ["integrations", wsId] as const,
  list: (wsId: string) => [...integrationKeys.all(wsId), "list"] as const,
};

export const integrationsOptions = (wsId: string) =>
  queryOptions({
    queryKey: integrationKeys.list(wsId),
    queryFn: () => api.listIntegrations(wsId),
    enabled: !!wsId,
  });
