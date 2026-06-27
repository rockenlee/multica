"use client";

import { useEffect, useState, type ReactNode } from "react";
import { BookOpen, Cloud, GitBranch, Kanban, Plus, Save, Trash2, Pencil, X } from "lucide-react";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { toast } from "sonner";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentWorkspace } from "@multica/core/paths";
import { memberListOptions, workspaceKeys } from "@multica/core/workspace/queries";
import {
  workspaceResourcesOptions,
  useCreateWorkspaceResource,
  useDeleteWorkspaceResource,
} from "@multica/core/projects";
import { api } from "@multica/core/api";
import type {
  ProjectResourceRef,
  WorkspaceResource,
  WorkspaceResourceType,
  Workspace,
  WorkspaceRepo,
} from "@multica/core/types";
import { useT } from "../../i18n";

type WorkspaceExternalResourceType = Extract<
  WorkspaceResourceType,
  "github_repo" | "gitlab_repo" | "feishu_drive" | "feishu_wiki" | "zentao_project" | "zentao_product"
>;

const WORKSPACE_RESOURCE_TYPES: WorkspaceExternalResourceType[] = [
  "github_repo",
  "gitlab_repo",
  "feishu_drive",
  "feishu_wiki",
  "zentao_project",
  "zentao_product",
];

function dropAndShiftIndex(set: Set<number>, removed: number): Set<number> {
  const next = new Set<number>();
  set.forEach((i) => {
    if (i === removed) return;
    next.add(i > removed ? i - 1 : i);
  });
  return next;
}

function isDirty(local: WorkspaceRepo[], saved: WorkspaceRepo[]): boolean {
  if (local.length !== saved.length) return true;
  return local.some((r, i) => r.url !== saved[i]?.url || (r.description ?? "") !== (saved[i]?.description ?? ""));
}

function isWorkspaceExternalResource(
  resource: WorkspaceResource,
): resource is WorkspaceResource & { resource_type: WorkspaceExternalResourceType } {
  return WORKSPACE_RESOURCE_TYPES.includes(
    resource.resource_type as WorkspaceExternalResourceType,
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function firstString(ref: ProjectResourceRef, keys: string[]): string | null {
  if (!isRecord(ref)) return null;
  for (const key of keys) {
    const value = ref[key];
    if (typeof value === "string" && value.trim()) {
      return value;
    }
  }
  return null;
}

function buildExternalResourceRef(
  resourceType: WorkspaceExternalResourceType,
  value: string,
  label: string,
): ProjectResourceRef {
  const trimmed = value.trim();
  const trimmedLabel = label.trim();
  const base = trimmedLabel ? { label: trimmedLabel } : {};

  if (resourceType === "github_repo" || resourceType === "gitlab_repo") {
    return { ...base, url: trimmed };
  }
  if (resourceType === "feishu_drive") {
    return trimmed.startsWith("http")
      ? { ...base, drive_url: trimmed }
      : { ...base, folder_token: trimmed };
  }
  if (resourceType === "feishu_wiki") {
    return trimmed.startsWith("http")
      ? { ...base, wiki_url: trimmed }
      : { ...base, space_id: trimmed };
  }
  if (resourceType === "zentao_project") {
    return trimmed.startsWith("http")
      ? { ...base, url: trimmed }
      : { ...base, project_id: trimmed };
  }
  return trimmed.startsWith("http")
    ? { ...base, url: trimmed }
    : { ...base, product_id: trimmed };
}

// Multi-type cards host two resource subtypes; the subtype is inferred from the
// value so the card stays a plain value+name form like Code repositories.
function detectGitType(value: string): WorkspaceExternalResourceType {
  return value.toLowerCase().includes("github") ? "github_repo" : "gitlab_repo";
}

function detectZentaoType(value: string): WorkspaceExternalResourceType {
  return value.toLowerCase().includes("product") ? "zentao_product" : "zentao_project";
}

function getResourceSummary(resource: WorkspaceResource): string {
  const fallback = resource.resource_type;
  const primary =
    resource.label ||
    firstString(resource.resource_ref, [
      "label",
      "drive_url",
      "wiki_url",
      "url",
      "folder_token",
      "space_id",
      "node_token",
      "project_key",
      "project_id",
      "product_key",
      "product_id",
    ]);
  return primary || fallback;
}

function getResourceDetail(resource: WorkspaceResource): string | null {
  return firstString(resource.resource_ref, [
    "url",
    "drive_url",
    "wiki_url",
    "folder_token",
    "space_id",
    "node_token",
    "project_key",
    "project_id",
    "product_key",
    "product_id",
  ]);
}

// A workspace-resource card that mirrors the Code repositories interaction:
// "+ Add" (left) reveals a value+name draft row, "Save" (right) persists, and
// each existing row has a delete action. Changes are batched and committed on
// Save (create new drafts, delete rows marked for removal).
function WorkspaceResourceCard({
  icon,
  title,
  description,
  resources,
  detectType,
  valuePlaceholder,
  resourceTypeLabels,
  canManage,
  onCreate,
  onDelete,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  resources: WorkspaceResource[];
  detectType: (value: string) => WorkspaceExternalResourceType;
  valuePlaceholder: string;
  resourceTypeLabels: Record<WorkspaceExternalResourceType, string>;
  canManage: boolean;
  onCreate: (input: {
    resource_type: WorkspaceExternalResourceType;
    resource_ref: ProjectResourceRef;
    label?: string;
  }) => Promise<unknown>;
  onDelete: (id: string) => Promise<unknown>;
}) {
  const { t } = useT("settings");
  const [drafts, setDrafts] = useState<{ value: string; label: string }[]>([]);
  const [pendingDeletes, setPendingDeletes] = useState<Set<string>>(new Set());
  const [saving, setSaving] = useState(false);

  const visibleResources = resources.filter((r) => !pendingDeletes.has(r.id));
  const dirty = drafts.some((d) => d.value.trim()) || pendingDeletes.size > 0;
  const isEmpty = visibleResources.length === 0 && drafts.length === 0;

  const handleSave = async () => {
    setSaving(true);
    try {
      for (const draft of drafts) {
        const value = draft.value.trim();
        if (!value) continue;
        const type = detectType(value);
        await onCreate({
          resource_type: type,
          resource_ref: buildExternalResourceRef(type, value, draft.label),
          label: draft.label.trim() || undefined,
        });
      }
      for (const id of pendingDeletes) {
        await onDelete(id);
      }
      setDrafts([]);
      setPendingDeletes(new Set());
      toast.success(t(($) => $.repositories.toast_saved));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.repositories.toast_save_failed));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex items-center gap-2">
          {icon}
          <h3 className="text-xs font-medium">{title}</h3>
        </div>
        <p className="text-xs text-muted-foreground">{description}</p>

        {isEmpty && (
          <p className="text-xs text-muted-foreground italic">
            {t(($) => $.repositories.workspace_resources_empty)}
          </p>
        )}

        {visibleResources.map((resource) => {
          const detail = getResourceDetail(resource);
          return (
            <div key={resource.id} className="group flex items-start gap-2">
              <div className="flex-1 min-w-0 rounded-md border bg-muted/50 px-3 py-2">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-xs font-medium">{getResourceSummary(resource)}</span>
                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    {resourceTypeLabels[resource.resource_type]}
                  </span>
                </div>
                {detail && (
                  <div
                    className="mt-0.5 truncate font-mono text-xs text-muted-foreground"
                    title={detail}
                  >
                    {detail}
                  </div>
                )}
              </div>
              {canManage && resource.can_manage && (
                <div className="flex shrink-0 items-center gap-0.5 pt-1.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100">
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t(($) => $.repositories.delete_resource_aria)}
                    className="text-muted-foreground hover:text-destructive"
                    disabled={saving}
                    onClick={() => setPendingDeletes((s) => new Set(s).add(resource.id))}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              )}
            </div>
          );
        })}

        {drafts.map((draft, index) => (
          <div key={`draft-${index}`} className="flex items-start gap-2">
            <div className="flex-1 min-w-0 space-y-1.5">
              <Input
                type="text"
                value={draft.value}
                onChange={(e) =>
                  setDrafts((d) =>
                    d.map((row, i) => (i === index ? { ...row, value: e.target.value } : row)),
                  )
                }
                placeholder={valuePlaceholder}
                className="text-sm"
              />
              <Input
                type="text"
                value={draft.label}
                onChange={(e) =>
                  setDrafts((d) =>
                    d.map((row, i) => (i === index ? { ...row, label: e.target.value } : row)),
                  )
                }
                placeholder={t(($) => $.repositories.resource_label_placeholder)}
                className="text-sm"
              />
            </div>
            <div className="flex shrink-0 items-center gap-0.5 pt-1.5">
              <Button
                variant="ghost"
                size="icon"
                aria-label={t(($) => $.repositories.cancel_aria)}
                className="text-muted-foreground hover:text-foreground"
                onClick={() => setDrafts((d) => d.filter((_, i) => i !== index))}
              >
                <X className="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        ))}

        {canManage ? (
          <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDrafts((d) => [...d, { value: "", label: "" }])}
            >
              <Plus className="h-3 w-3" />
              {t(($) => $.repositories.add_resource)}
            </Button>
            <div className="flex items-center gap-3">
              {!dirty && visibleResources.length > 0 && (
                <span className="text-xs text-muted-foreground">
                  {t(($) => $.repositories.saved_hint)}
                </span>
              )}
              <Button size="sm" onClick={handleSave} disabled={saving || !dirty}>
                <Save className="h-3 w-3" />
                {saving ? t(($) => $.repositories.saving) : t(($) => $.repositories.save)}
              </Button>
            </div>
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t(($) => $.repositories.manage_resource_hint)}
          </p>
        )}
      </CardContent>
    </Card>
  );
}

export function RepositoriesTab() {
  const { t } = useT("settings");
  const user = useAuthStore((s) => s.user);
  const workspace = useCurrentWorkspace();
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: workspaceResources = [] } = useQuery(workspaceResourcesOptions(wsId));
  const createWorkspaceResource = useCreateWorkspaceResource(wsId);
  const deleteWorkspaceResource = useDeleteWorkspaceResource(wsId);

  const [repos, setRepos] = useState<WorkspaceRepo[]>(workspace?.repos ?? []);
  const [editingIndices, setEditingIndices] = useState<Set<number>>(new Set());
  const [saving, setSaving] = useState(false);

  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManageWorkspace = currentMember?.role === "owner" || currentMember?.role === "admin";

  useEffect(() => {
    setRepos(workspace?.repos ?? []);
  }, [workspace]);

  const savedRepos = workspace?.repos ?? [];
  const dirty = isDirty(repos, savedRepos);
  const externalResources = workspaceResources.filter(isWorkspaceExternalResource);
  const resourceTypeLabels: Record<WorkspaceExternalResourceType, string> = {
    github_repo: t(($) => $.repositories.github_repo_type),
    gitlab_repo: t(($) => $.repositories.gitlab_repo_type),
    feishu_drive: t(($) => $.repositories.feishu_drive_type),
    feishu_wiki: t(($) => $.repositories.feishu_wiki_type),
    zentao_project: t(($) => $.repositories.zentao_project_type),
    zentao_product: t(($) => $.repositories.zentao_product_type),
  };
  const gitResources = externalResources.filter(
    (resource) => resource.resource_type === "github_repo" || resource.resource_type === "gitlab_repo",
  );
  const feishuDriveResources = externalResources.filter(
    (resource) => resource.resource_type === "feishu_drive",
  );
  const feishuWikiResources = externalResources.filter(
    (resource) => resource.resource_type === "feishu_wiki",
  );
  const zentaoResources = externalResources.filter(
    (resource) => resource.resource_type === "zentao_project" || resource.resource_type === "zentao_product",
  );

  const handleSave = async () => {
    if (!workspace) return;
    setSaving(true);
    try {
      const updated = await api.updateWorkspace(workspace.id, { repos });
      qc.setQueryData(workspaceKeys.list(), (old: Workspace[] | undefined) =>
        old?.map((ws) => (ws.id === updated.id ? updated : ws)),
      );
      setEditingIndices(new Set());
      toast.success(t(($) => $.repositories.toast_saved));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.repositories.toast_save_failed));
    } finally {
      setSaving(false);
    }
  };

  const handleAddRepo = () => {
    const nextIndex = repos.length;
    setRepos([...repos, { url: "" }]);
    setEditingIndices(new Set(editingIndices).add(nextIndex));
  };

  const handleRemoveRepo = (index: number) => {
    setRepos(repos.filter((_, i) => i !== index));
    setEditingIndices(dropAndShiftIndex(editingIndices, index));
  };

  const handleRepoChange = (index: number, field: keyof WorkspaceRepo, value: string) => {
    setRepos(repos.map((r, i) => (i === index ? { ...r, [field]: value } : r)));
  };

  const handleEditRepo = (index: number) => {
    setEditingIndices(new Set(editingIndices).add(index));
  };

  const handleCancelEdit = (index: number) => {
    const saved = savedRepos[index];
    if (saved === undefined) {
      // Newly added row that was never persisted — drop it entirely.
      handleRemoveRepo(index);
      return;
    }
    setRepos(repos.map((r, i) => (i === index ? { ...r, url: saved.url, description: saved.description } : r)));
    const next = new Set(editingIndices);
    next.delete(index);
    setEditingIndices(next);
  };

  if (!workspace) return null;

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <h2 className="text-sm font-semibold">{t(($) => $.repositories.section_title)}</h2>
        <p className="text-xs text-muted-foreground">
          {t(($) => $.repositories.resource_description)}
        </p>

        <Card>
          <CardContent className="space-y-3">
            <div className="flex items-center gap-2">
              <GitBranch className="h-3.5 w-3.5 text-muted-foreground" />
              <h3 className="text-xs font-medium">{t(($) => $.repositories.code_section_title)}</h3>
            </div>
            <p className="text-xs text-muted-foreground">
              {t(($) => $.repositories.description)}
            </p>

            {repos.length === 0 && (
              <p className="text-xs text-muted-foreground italic">
                {t(($) => $.repositories.empty)}
              </p>
            )}

            {repos.map((repo, index) => {
              const isEditing = editingIndices.has(index);
              return (
                <div
                  key={index}
                  className="group flex items-start gap-2"
                >
                  {isEditing ? (
                    <div className="flex-1 min-w-0 space-y-1.5">
                      <Input
                        type="text"
                        value={repo.url}
                        onChange={(e) => handleRepoChange(index, "url", e.target.value)}
                        disabled={!canManageWorkspace}
                        placeholder={t(($) => $.repositories.url_placeholder)}
                        className="text-sm"
                      />
                      <Input
                        type="text"
                        value={repo.description ?? ""}
                        onChange={(e) => handleRepoChange(index, "description", e.target.value)}
                        disabled={!canManageWorkspace}
                        placeholder={t(($) => $.repositories.description_placeholder)}
                        className="text-sm"
                      />
                    </div>
                  ) : (
                    <div className="flex-1 min-w-0 rounded-md border bg-muted/50 px-3 py-2">
                      <div
                        className="truncate font-mono text-xs text-muted-foreground"
                        title={repo.url}
                      >
                        {repo.url || t(($) => $.repositories.url_empty)}
                      </div>
                      {repo.description && (
                        <div className="mt-0.5 truncate text-xs text-muted-foreground/70" title={repo.description}>
                          {repo.description}
                        </div>
                      )}
                    </div>
                  )}
                  {canManageWorkspace && (
                    <div
                      className={
                        isEditing
                          ? "flex shrink-0 items-center gap-0.5 pt-1.5"
                          : "flex shrink-0 items-center gap-0.5 pt-1.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100"
                      }
                    >
                      {!isEditing && (
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={t(($) => $.repositories.edit_aria)}
                          className="text-muted-foreground hover:text-foreground"
                          onClick={() => handleEditRepo(index)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      {isEditing && (
                        <Button
                          variant="ghost"
                          size="icon"
                          aria-label={t(($) => $.repositories.cancel_aria)}
                          className="text-muted-foreground hover:text-foreground"
                          onClick={() => handleCancelEdit(index)}
                        >
                          <X className="h-3.5 w-3.5" />
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t(($) => $.repositories.delete_aria)}
                        className="text-muted-foreground hover:text-destructive"
                        onClick={() => handleRemoveRepo(index)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  )}
                </div>
              );
            })}

            {canManageWorkspace && (
              <div className="flex flex-wrap items-center justify-between gap-2 pt-1">
                <Button variant="outline" size="sm" onClick={handleAddRepo}>
                  <Plus className="h-3 w-3" />
                  {t(($) => $.repositories.add)}
                </Button>
                <div className="flex items-center gap-3">
                  {!dirty && repos.length > 0 && (
                    <span className="text-xs text-muted-foreground">
                      {t(($) => $.repositories.saved_hint)}
                    </span>
                  )}
                  <Button
                    size="sm"
                    onClick={handleSave}
                    disabled={saving || !dirty}
                  >
                    <Save className="h-3 w-3" />
                    {saving ? t(($) => $.repositories.saving) : t(($) => $.repositories.save)}
                  </Button>
                </div>
              </div>
            )}

            {!canManageWorkspace && (
              <p className="text-xs text-muted-foreground">
                {t(($) => $.repositories.manage_hint)}
              </p>
            )}
          </CardContent>
        </Card>

        <WorkspaceResourceCard
          icon={<GitBranch className="h-3.5 w-3.5 text-muted-foreground" />}
          title={t(($) => $.repositories.git_section_title)}
          description={t(($) => $.repositories.git_description)}
          resources={gitResources}
          detectType={detectGitType}
          valuePlaceholder={t(($) => $.repositories.git_value_placeholder)}
          resourceTypeLabels={resourceTypeLabels}
          canManage={canManageWorkspace}
          onCreate={(input) => createWorkspaceResource.mutateAsync(input)}
          onDelete={(id) => deleteWorkspaceResource.mutateAsync(id)}
        />

        <WorkspaceResourceCard
          icon={<Cloud className="h-3.5 w-3.5 text-muted-foreground" />}
          title={t(($) => $.repositories.lark_drive_section_title)}
          description={t(($) => $.repositories.lark_drive_description)}
          resources={feishuDriveResources}
          detectType={() => "feishu_drive"}
          valuePlaceholder={t(($) => $.repositories.feishu_drive_placeholder)}
          resourceTypeLabels={resourceTypeLabels}
          canManage={canManageWorkspace}
          onCreate={(input) => createWorkspaceResource.mutateAsync(input)}
          onDelete={(id) => deleteWorkspaceResource.mutateAsync(id)}
        />

        <WorkspaceResourceCard
          icon={<BookOpen className="h-3.5 w-3.5 text-muted-foreground" />}
          title={t(($) => $.repositories.wiki_section_title)}
          description={t(($) => $.repositories.wiki_description)}
          resources={feishuWikiResources}
          detectType={() => "feishu_wiki"}
          valuePlaceholder={t(($) => $.repositories.feishu_wiki_placeholder)}
          resourceTypeLabels={resourceTypeLabels}
          canManage={canManageWorkspace}
          onCreate={(input) => createWorkspaceResource.mutateAsync(input)}
          onDelete={(id) => deleteWorkspaceResource.mutateAsync(id)}
        />

        <WorkspaceResourceCard
          icon={<Kanban className="h-3.5 w-3.5 text-muted-foreground" />}
          title={t(($) => $.repositories.zentao_section_title)}
          description={t(($) => $.repositories.zentao_description)}
          resources={zentaoResources}
          detectType={detectZentaoType}
          valuePlaceholder={t(($) => $.repositories.zentao_value_placeholder)}
          resourceTypeLabels={resourceTypeLabels}
          canManage={canManageWorkspace}
          onCreate={(input) => createWorkspaceResource.mutateAsync(input)}
          onDelete={(id) => deleteWorkspaceResource.mutateAsync(id)}
        />
      </section>
    </div>
  );
}
