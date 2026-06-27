"use client";

import { getIssueSourceProvider, type IssueSyncProvider } from "@multica/core/issues";
import type { Issue } from "@multica/core/types";
import { useT } from "../../i18n";

const SOURCE_LABEL_KEY: Record<IssueSyncProvider, "source_feishu" | "source_zentao" | "source_gitlab"> = {
  feishu: "source_feishu",
  zentao: "source_zentao",
  gitlab: "source_gitlab",
};

export function IssueSourceBadge({ issue }: { issue: Issue }) {
  const { t } = useT("issues");
  const provider = getIssueSourceProvider(issue);
  if (!provider) return null;

  return (
    <span className="inline-flex shrink-0 items-center rounded-sm bg-muted px-1.5 py-0.5 text-[10px] font-medium leading-none text-muted-foreground">
      {t(($) => $.sync[SOURCE_LABEL_KEY[provider]])}
    </span>
  );
}
