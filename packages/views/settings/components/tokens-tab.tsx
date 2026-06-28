"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { Key, Trash2, Copy, Check } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Tooltip, TooltipTrigger, TooltipContent } from "@multica/ui/components/ui/tooltip";
import type { IntegrationConnection, IntegrationUserAccount, PersonalAccessToken } from "@multica/core/types";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@multica/ui/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { copyText } from "@multica/ui/lib/clipboard";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { integrationsOptions, integrationKeys } from "@multica/core/integrations";
import { useWorkspaceId } from "@multica/core/hooks";
import { useT } from "../../i18n";
import { ConnectionStatusBadge, type ConnectionStatus } from "./connection-status";

const EXPIRY_KEYS = ["30", "90", "365", "never"] as const;
const FEISHU_OAUTH_ACCOUNT_KEY = "feishuuseroauth";
const EMPTY_CONNECTIONS: IntegrationConnection[] = [];
const EMPTY_ACCOUNTS: IntegrationUserAccount[] = [];

function normalizeExternalIdentity(value: unknown) {
  return typeof value === "string" ? value.trim().toLowerCase() : "";
}

function normalizeAccountKeyIdentity(value: unknown) {
  return normalizeExternalIdentity(value).replace(/[-_\s]/g, "");
}

function configString(config: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = config[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return "";
}

function isFeishuOAuthUserAccount(
  connection: IntegrationConnection,
  account: IntegrationUserAccount,
) {
  if (connection.provider !== "feishu") return false;
  return normalizeAccountKeyIdentity(account.account_key) === FEISHU_OAUTH_ACCOUNT_KEY;
}

function feishuOAuthDedupeKey(
  connection: IntegrationConnection,
  account: IntegrationUserAccount,
) {
  const appIdentity = configString(connection.config, ["app_id", "appId", "client_id", "clientId"]) || connection.id;
  const userIdentity =
    normalizeAccountKeyIdentity(account.external_user_id) ||
    normalizeAccountKeyIdentity(account.external_username) ||
    normalizeAccountKeyIdentity(account.account_key);
  return `${connection.provider}:${normalizeExternalIdentity(appIdentity)}:${userIdentity}`;
}

function visibleExternalAccountsByConnection(
  connections: IntegrationConnection[],
  accounts: IntegrationUserAccount[],
) {
  const seenFeishuOAuth = new Set<string>();
  const byConnection = new Map<string, IntegrationUserAccount[]>();

  for (const connection of connections) {
    const visible: IntegrationUserAccount[] = [];
    for (const account of accounts.filter((item) => item.connection_id === connection.id)) {
      if (isFeishuOAuthUserAccount(connection, account)) {
        const key = feishuOAuthDedupeKey(connection, account);
        if (seenFeishuOAuth.has(key)) continue;
        seenFeishuOAuth.add(key);
      }
      visible.push(account);
    }
    byConnection.set(connection.id, visible);
  }

  return byConnection;
}

export function TokensTab() {
  const { t } = useT("settings");
  const [tokens, setTokens] = useState<PersonalAccessToken[]>([]);
  const [tokenName, setTokenName] = useState("");
  const [tokenExpiry, setTokenExpiry] = useState("90");
  const [tokenCreating, setTokenCreating] = useState(false);
  const [newToken, setNewToken] = useState<string | null>(null);
  const [tokenCopied, setTokenCopied] = useState(false);
  const [tokenRevoking, setTokenRevoking] = useState<string | null>(null);
  const [revokeConfirmId, setRevokeConfirmId] = useState<string | null>(null);
  const [tokensLoading, setTokensLoading] = useState(true);

  const loadTokens = useCallback(async () => {
    try {
      const list = await api.listPersonalAccessTokens();
      setTokens(list);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.tokens.toast_load_failed));
    } finally {
      setTokensLoading(false);
    }
  }, [t]);

  useEffect(() => { loadTokens(); }, [loadTokens]);

  const handleCreateToken = async () => {
    setTokenCreating(true);
    try {
      const expiresInDays = tokenExpiry === "never" ? undefined : Number(tokenExpiry);
      const result = await api.createPersonalAccessToken({ name: tokenName, expires_in_days: expiresInDays });
      setNewToken(result.token);
      setTokenName("");
      setTokenExpiry("90");
      await loadTokens();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.tokens.toast_create_failed));
    } finally {
      setTokenCreating(false);
    }
  };

  const handleRevokeToken = async (id: string) => {
    setTokenRevoking(id);
    try {
      await api.revokePersonalAccessToken(id);
      await loadTokens();
      toast.success(t(($) => $.tokens.toast_revoked));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.tokens.toast_revoke_failed));
    } finally {
      setTokenRevoking(null);
    }
  };

  const handleCopyToken = async () => {
    if (!newToken) return;
    if (await copyText(newToken)) {
      setTokenCopied(true);
      setTimeout(() => setTokenCopied(false), 2000);
    }
  };

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <div className="flex items-center gap-2">
          <Key className="h-4 w-4 text-muted-foreground" />
          <h2 className="text-sm font-semibold">{t(($) => $.tokens.title)}</h2>
        </div>

        <Card>
          <CardContent className="space-y-3">
            <p className="text-xs text-muted-foreground">
              {t(($) => $.tokens.description)}
            </p>
            <div className="grid gap-3 sm:grid-cols-[1fr_120px_auto]">
              <Input
                type="text"
                value={tokenName}
                onChange={(e) => setTokenName(e.target.value)}
                placeholder={t(($) => $.tokens.name_placeholder)}
              />
              <Select value={tokenExpiry} onValueChange={(v) => { if (v) setTokenExpiry(v); }}>
                <SelectTrigger size="sm"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {EXPIRY_KEYS.map((key) => (
                    <SelectItem key={key} value={key}>{t(($) => $.tokens.expiry[key])}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button onClick={handleCreateToken} disabled={tokenCreating || !tokenName.trim()}>
                {tokenCreating ? t(($) => $.tokens.creating) : t(($) => $.tokens.create)}
              </Button>
            </div>
          </CardContent>
        </Card>

        {tokensLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 2 }).map((_, i) => (
              <Card key={i}>
                <CardContent className="flex items-center gap-3">
                  <div className="flex-1 space-y-1.5">
                    <Skeleton className="h-4 w-32" />
                    <Skeleton className="h-3 w-48" />
                  </div>
                  <Skeleton className="h-8 w-8 rounded" />
                </CardContent>
              </Card>
            ))}
          </div>
        ) : tokens.length > 0 && (
          <div className="space-y-2">
            {tokens.map((token) => (
              <Card key={token.id}>
                <CardContent className="flex items-center gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium truncate">{token.name}</div>
                    <div className="text-xs text-muted-foreground">
                      {t(($) => $.tokens.metadata_prefix, {
                        prefix: token.token_prefix,
                        created: new Date(token.created_at).toLocaleDateString(),
                        lastUsed: token.last_used_at
                          ? t(($) => $.tokens.last_used_with_date, {
                              date: new Date(token.last_used_at!).toLocaleDateString(),
                            })
                          : t(($) => $.tokens.last_used_never),
                      })}
                      {token.expires_at && t(($) => $.tokens.expires_with_date, {
                        date: new Date(token.expires_at!).toLocaleDateString(),
                      })}
                    </div>
                  </div>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => setRevokeConfirmId(token.id)}
                          disabled={tokenRevoking === token.id}
                          aria-label={t(($) => $.tokens.revoke_aria, { name: token.name })}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      }
                    />
                    <TooltipContent>{t(($) => $.tokens.revoke_tooltip)}</TooltipContent>
                  </Tooltip>
                </CardContent>
              </Card>
            ))}
          </div>
        )}
      </section>

      <AlertDialog open={!!revokeConfirmId} onOpenChange={(v) => { if (!v) setRevokeConfirmId(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($) => $.tokens.revoke_dialog.title)}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.tokens.revoke_dialog.description)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t(($) => $.tokens.revoke_dialog.cancel)}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={async () => {
                if (revokeConfirmId) await handleRevokeToken(revokeConfirmId);
                setRevokeConfirmId(null);
              }}
            >
              {t(($) => $.tokens.revoke_dialog.confirm)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={!!newToken} onOpenChange={(v) => { if (!v) { setNewToken(null); setTokenCopied(false); } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(($) => $.tokens.created_dialog.title)}</DialogTitle>
            <DialogDescription>
              {t(($) => $.tokens.created_dialog.description)}
            </DialogDescription>
          </DialogHeader>
          <div className="flex items-center gap-2">
            <code className="flex-1 rounded-md border bg-muted/50 px-3 py-2 text-sm break-all select-all">
              {newToken}
            </code>
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button variant="outline" size="icon" onClick={handleCopyToken}>
                    {tokenCopied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
                  </Button>
                }
              />
              <TooltipContent>{t(($) => $.tokens.created_dialog.copy_tooltip)}</TooltipContent>
            </Tooltip>
          </div>
          <DialogFooter>
            <Button onClick={() => { setNewToken(null); setTokenCopied(false); }}>{t(($) => $.tokens.created_dialog.done)}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ExternalAccountsSection />
    </div>
  );
}

function ExternalAccountsSection() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const { data, isLoading } = useQuery(integrationsOptions(wsId));
  const connections = data?.connections ?? EMPTY_CONNECTIONS;
  const accounts = data?.accounts ?? EMPTY_ACCOUNTS;
  const visibleAccounts = useMemo(
    () => visibleExternalAccountsByConnection(connections, accounts),
    [connections, accounts],
  );

  return (
    <section className="space-y-4">
      <div className="flex items-center gap-2">
        <Key className="h-4 w-4 text-muted-foreground" />
        <h2 className="text-sm font-semibold">
          {t(($) => $.tokens.external_accounts_title)}
        </h2>
      </div>
      <Card>
        <CardContent className="space-y-3">
          <p className="text-xs text-muted-foreground">
            {t(($) => $.tokens.external_accounts_description)}
          </p>
          {isLoading ? (
            <p className="text-sm text-muted-foreground">{t(($) => $.lark.loading)}</p>
          ) : connections.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t(($) => $.tokens.external_accounts_empty)}
            </p>
          ) : (
            <div className="space-y-4">
              {connections.map((connection) => {
                const connectionAccounts = visibleAccounts.get(connection.id) ?? [];
                return (
                  <div key={connection.id} className="space-y-2">
                    {connectionAccounts.map((account) => (
                      <ExternalAccountRow
                        key={account.id}
                        connection={connection}
                        account={account}
                        credentialStorageEnabled={data?.credential_storage_enabled === true}
                      />
                    ))}
                    <ExternalAccountRow
                      key={`${connection.id}-new`}
                      connection={connection}
                      account={null}
                      credentialStorageEnabled={data?.credential_storage_enabled === true}
                    />
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </section>
  );
}

function ExternalAccountRow({
  connection,
  account,
  credentialStorageEnabled,
}: {
  connection: IntegrationConnection;
  account: IntegrationUserAccount | null;
  credentialStorageEnabled: boolean;
}) {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const [accountName, setAccountName] = useState(account?.account_name ?? "");
  const [username, setUsername] = useState(account?.external_username ?? "");
  const [scopes, setScopes] = useState(account?.scopes?.join(", ") ?? "");
  const [expiresAt, setExpiresAt] = useState(account?.expires_at?.slice(0, 10) ?? "");
  const [credential, setCredential] = useState("");
  const [syncEnabled, setSyncEnabled] = useState(account?.sync_enabled === true);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [password, setPassword] = useState("");
  const [connecting, setConnecting] = useState(false);
  const [disconnectOpen, setDisconnectOpen] = useState(false);

  const isFeishu = connection.provider === "feishu";
  const isZentao = connection.provider === "zentao";

  // Connection status derived from the stored account, surfaced as a badge so
  // the user can see at a glance whether a re-connect is needed.
  const connStatus: ConnectionStatus = !account?.credential_configured
    ? "not_configured"
    : account.last_error || account.status === "error"
      ? "error"
      : account.expires_at && new Date(account.expires_at).getTime() < Date.now()
        ? "action_needed"
        : "connected";
  const statusLabel =
    connStatus === "connected"
      ? t(($) => $.integrations.status.connected)
      : connStatus === "action_needed"
        ? t(($) => $.integrations.status.action_needed)
        : connStatus === "error"
          ? t(($) => $.integrations.status.error)
          : "";

  async function connectFeishu() {
    if (connecting) return;
    setConnecting(true);
    try {
      const { authorize_url } = await api.startFeishuOAuth(wsId, connection.id);
      window.open(authorize_url, "_blank", "noopener,noreferrer");
      toast.info(t(($) => $.tokens.external_accounts_feishu_connect_opened));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.tokens.external_accounts_save_failed));
    } finally {
      setConnecting(false);
    }
  }

  async function connectZentao() {
    if (connecting || !username.trim() || !password.trim()) return;
    setConnecting(true);
    try {
      await api.zentaoLogin(wsId, connection.id, {
        account: username.trim(),
        password: password.trim(),
        account_key: account?.account_key ?? undefined,
        account_name: accountName.trim() || username.trim(),
      });
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
      setPassword("");
      toast.success(t(($) => $.tokens.external_accounts_zentao_connected));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.tokens.external_accounts_save_failed));
    } finally {
      setConnecting(false);
    }
  }

  useEffect(() => {
    setAccountName(account?.account_name ?? "");
    setUsername(account?.external_username ?? "");
    setScopes(account?.scopes?.join(", ") ?? "");
    setExpiresAt(account?.expires_at?.slice(0, 10) ?? "");
    setCredential("");
    setSyncEnabled(account?.sync_enabled === true);
  }, [
    account?.account_name,
    account?.external_username,
    account?.expires_at,
    account?.scopes,
    account?.sync_enabled,
  ]);

  async function saveAccount() {
    if (saving || !accountName.trim()) return;
    setSaving(true);
    try {
      await api.upsertIntegrationUserAccount(wsId, connection.id, {
        account_key: account?.account_key ?? undefined,
        account_name: accountName.trim(),
        external_username: username.trim() || null,
        credential: credential.trim() || undefined,
        scopes: scopes
          .split(",")
          .map((scope) => scope.trim())
          .filter(Boolean),
        config: {},
        status: "active",
        sync_enabled: syncEnabled,
        expires_at: expiresAt.trim() || null,
      });
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
      setCredential("");
      toast.success(t(($) => $.tokens.external_accounts_saved));
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.tokens.external_accounts_save_failed),
      );
    } finally {
      setSaving(false);
    }
  }

  async function deleteAccount() {
    if (!account || deleting) return;
    setDeleting(true);
    try {
      await api.deleteIntegrationUserAccountById(wsId, account.id);
      await qc.invalidateQueries({ queryKey: integrationKeys.list(wsId) });
      toast.success(t(($) => $.tokens.external_accounts_deleted));
    } catch (e) {
      toast.error(
        e instanceof Error
          ? e.message
          : t(($) => $.tokens.external_accounts_delete_failed),
      );
    } finally {
      setDeleting(false);
    }
  }

  return (
    <div className="space-y-3 rounded-md border bg-muted/30 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <div className="text-sm font-medium">
            {account?.account_name || t(($) => $.tokens.external_accounts_new_title, { provider: connection.name })}
          </div>
          <div className="text-xs text-muted-foreground">
            {connection.base_url || connection.provider}
          </div>
        </div>
        {connStatus !== "not_configured" && (
          <ConnectionStatusBadge status={connStatus} label={statusLabel} />
        )}
      </div>
      <div className="grid gap-2 md:grid-cols-2">
        <div className="space-y-1.5">
          <Label className="text-xs">
            {t(($) => $.tokens.external_accounts_account_name)}
          </Label>
          <Input
            value={accountName}
            onChange={(event) => setAccountName(event.target.value)}
            placeholder={t(($) => $.tokens.external_accounts_account_name_placeholder)}
          />
        </div>
        <div className="space-y-1.5">
          <Label className="text-xs">
            {t(($) => $.tokens.external_accounts_username)}
          </Label>
          <Input
            value={username}
            onChange={(event) => setUsername(event.target.value)}
            placeholder={t(($) => $.tokens.external_accounts_username_placeholder)}
          />
        </div>
        {isZentao ? (
          <div className="space-y-1.5">
            <Label className="text-xs">
              {t(($) => $.tokens.external_accounts_zentao_password)}
            </Label>
            <Input
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder={t(($) => $.tokens.external_accounts_zentao_password_placeholder)}
              disabled={!credentialStorageEnabled}
            />
          </div>
        ) : isFeishu ? null : (
          <div className="space-y-1.5">
            <Label className="text-xs">
              {t(($) => $.tokens.external_accounts_token)}
            </Label>
            <Input
              type="password"
              value={credential}
              onChange={(event) => setCredential(event.target.value)}
              placeholder={
                account?.credential_configured
                  ? t(($) => $.tokens.external_accounts_token_configured)
                  : t(($) => $.tokens.external_accounts_token_placeholder)
              }
              disabled={!credentialStorageEnabled}
            />
          </div>
        )}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <Switch
            id={`external-account-${connection.id}`}
            size="sm"
            checked={syncEnabled}
            onCheckedChange={setSyncEnabled}
          />
          <Label
            htmlFor={`external-account-${connection.id}`}
            className="text-xs text-muted-foreground"
          >
            {t(($) => $.tokens.external_accounts_sync)}
          </Label>
        </div>
        <div className="flex items-center gap-2">
          {isFeishu && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={connectFeishu}
              disabled={connecting || !credentialStorageEnabled}
            >
              {account?.credential_configured
                ? t(($) => $.integrations.reconnect)
                : t(($) => $.tokens.external_accounts_feishu_connect)}
            </Button>
          )}
          {isZentao && (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={connectZentao}
              disabled={
                connecting ||
                !username.trim() ||
                !password.trim() ||
                !credentialStorageEnabled
              }
            >
              {connecting
                ? t(($) => $.integrations.connecting)
                : account?.credential_configured
                  ? t(($) => $.integrations.reconnect)
                  : t(($) => $.tokens.external_accounts_zentao_connect)}
            </Button>
          )}
          <Button
            type="button"
            size="sm"
            onClick={saveAccount}
            disabled={
              saving ||
              !accountName.trim() ||
              (!credentialStorageEnabled && credential.trim().length > 0)
            }
          >
            {saving
              ? t(($) => $.tokens.external_accounts_saving)
              : t(($) => $.tokens.external_accounts_save)}
          </Button>
          {account && (
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setDisconnectOpen(true)}
                    disabled={deleting}
                    aria-label={t(($) => $.tokens.external_accounts_delete_aria, {
                      name: account.account_name || connection.name,
                    })}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                }
              />
              <TooltipContent>{t(($) => $.tokens.external_accounts_remove_tooltip)}</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>
      <AlertDialog open={disconnectOpen} onOpenChange={setDisconnectOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($) => $.tokens.external_accounts_remove_title)}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.tokens.external_accounts_remove_desc)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t(($) => $.tokens.revoke_dialog.cancel)}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={async () => {
                setDisconnectOpen(false);
                await deleteAccount();
              }}
            >
              {t(($) => $.tokens.external_accounts_remove)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      {isFeishu && (
        <p className="text-xs text-muted-foreground">
          {t(($) => $.tokens.external_accounts_feishu_connect_hint)}
        </p>
      )}
      {!credentialStorageEnabled && (
        <p className="text-xs text-muted-foreground">
          {t(($) => $.tokens.external_accounts_secret_required)}
        </p>
      )}
    </div>
  );
}
