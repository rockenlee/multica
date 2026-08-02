"use client";

// Canonical connection-status primitive shared by the admin (Integrations) and
// member (Access Tokens) layers so every provider shows the same four states
// with the same vocabulary and colors. Callers pass the resolved label so this
// stays i18n-agnostic.
export type ConnectionStatus =
  | "not_configured"
  | "action_needed"
  | "connected"
  | "error";

const STATUS_CLASS: Record<ConnectionStatus, string> = {
  connected: "bg-green-500/15 text-green-600",
  action_needed: "bg-amber-500/15 text-amber-700",
  error: "bg-red-500/15 text-red-600",
  not_configured: "bg-muted text-muted-foreground",
};

export function ConnectionStatusBadge({
  status,
  label,
}: {
  status: ConnectionStatus;
  label: string;
}) {
  return (
    <span className={`rounded px-1.5 py-0.5 text-micro ${STATUS_CLASS[status]}`}>
      {label}
    </span>
  );
}
