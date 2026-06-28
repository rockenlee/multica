"use client";

import { useState } from "react";
import { Check, Copy, Terminal } from "lucide-react";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { CODE_LIGATURE_CLASS } from "@multica/ui/lib/code-style";
import { cn } from "@multica/ui/lib/utils";
import { copyText } from "@multica/ui/lib/clipboard";
import { useConfigStore } from "@multica/core/config";
import { useT } from "../../i18n";

const CLOUD_RELEASE_REPO = "multica-ai/multica";
const SELF_HOST_RELEASE_REPO = "rockenlee/multica";
const CLOUD_HOSTS = ["multica.ai", "www.multica.ai"];

function currentOriginSafe(): string {
  return typeof window === "undefined" ? "" : window.location.origin;
}

function normalizeCommandURL(url: string | undefined) {
  return url?.trim().replace(/\/+$/, "") ?? "";
}

function isManagedCloudOrigin(origin: string): boolean {
  try {
    const host = new URL(origin).hostname;
    return CLOUD_HOSTS.includes(host) || host.endsWith(".multica.ai");
  } catch {
    return false;
  }
}

function cliCommands(
  serverUrl: string | undefined,
  appUrl: string | undefined,
  currentOrigin: string,
) {
  const normalizedServerUrl = normalizeCommandURL(serverUrl);
  const normalizedAppUrl = normalizeCommandURL(appUrl);
  const currentOriginIsCloud = !currentOrigin || isManagedCloudOrigin(currentOrigin);
  const releaseRepo = currentOriginIsCloud ? CLOUD_RELEASE_REPO : SELF_HOST_RELEASE_REPO;
  const installCmd = `curl -fsSL https://raw.githubusercontent.com/${releaseRepo}/main/scripts/install.sh | bash`;

  if (normalizedServerUrl && normalizedAppUrl) {
    return {
      installCmd,
      setupCmd: `multica setup self-host --server-url ${normalizedServerUrl} --app-url ${normalizedAppUrl}`,
    };
  }

  if (currentOriginIsCloud) {
    return { installCmd, setupCmd: "multica setup" };
  }

  const origin = normalizeCommandURL(currentOrigin);
  return {
    installCmd,
    setupCmd: `multica setup self-host --server-url ${origin} --app-url ${origin}`,
  };
}

function CopyButton({ text }: { text: string }) {
  const { t } = useT("onboarding");
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    void copyText(text).then((ok) => {
      if (!ok) return;
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  };

  return (
    <button
      type="button"
      onClick={handleCopy}
      className="shrink-0 rounded p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      aria-label={t(($) => $.cli_install.copy_aria)}
    >
      {copied ? (
        <Check className="h-3.5 w-3.5 text-success" />
      ) : (
        <Copy className="h-3.5 w-3.5" />
      )}
    </button>
  );
}

function Step({ n, label, cmd }: { n: number; label: string; cmd: string }) {
  return (
    <div>
      <p className="mb-1.5 text-xs font-medium text-foreground">
        {n}. {label}
      </p>
      <div className="flex items-start gap-2 rounded-lg bg-muted px-3 py-2.5 font-mono text-sm">
        <Terminal className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <code
          className={cn(
            "min-w-0 flex-1 whitespace-pre-wrap break-all",
            CODE_LIGATURE_CLASS,
          )}
        >
          {cmd}
        </code>
        <CopyButton text={cmd} />
      </div>
    </div>
  );
}

export function CliInstallInstructions() {
  const { t } = useT("onboarding");
  const daemonServerUrl = useConfigStore((s) => s.daemonServerUrl);
  const daemonAppUrl = useConfigStore((s) => s.daemonAppUrl);
  const { installCmd, setupCmd } = cliCommands(
    daemonServerUrl,
    daemonAppUrl,
    currentOriginSafe(),
  );
  return (
    <Card className="w-full">
      <CardContent className="space-y-4 pt-4">
        <p className="text-xs leading-[1.55] text-muted-foreground">
          {t(($) => $.cli_install.intro)}
        </p>
        <Step n={1} label={t(($) => $.cli_install.step1_label)} cmd={installCmd} />
        <Step n={2} label={t(($) => $.cli_install.step2_label)} cmd={setupCmd} />
      </CardContent>
    </Card>
  );
}
