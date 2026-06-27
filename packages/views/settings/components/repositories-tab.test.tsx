import type { ReactNode } from "react";
import { describe, it, expect, beforeEach, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enSettings from "../../locales/en/settings.json";

const mockUpdateWorkspace = vi.hoisted(() => vi.fn());
const mockCreateWorkspaceResource = vi.hoisted(() => vi.fn());
const mockDeleteWorkspaceResource = vi.hoisted(() => vi.fn());
const mockInvalidateQueries = vi.hoisted(() => vi.fn());
const workspaceRef = vi.hoisted(() => ({
  current: {
    id: "workspace-1",
    name: "Test Workspace",
    slug: "test-workspace",
    repos: [{ url: "https://github.com/multica-ai/multica" }] as { url: string; description?: string }[],
  },
}));
const membersRef = vi.hoisted(() => ({
  current: [{ user_id: "user-1", role: "owner" as const }],
}));
const workspaceResourcesRef = vi.hoisted(() => ({
  current: [] as {
    id: string;
    workspace_id: string;
    resource_type: string;
    resource_ref: Record<string, unknown>;
    label: string | null;
    created_at: string;
    updated_at: string;
    created_by: string | null;
    can_manage: boolean;
  }[],
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: (options?: { queryKey?: readonly unknown[] }) => {
    const key = options?.queryKey;
    if (Array.isArray(key) && key[0] === "workspaces" && key.includes("resources")) {
      return { data: workspaceResourcesRef.current };
    }
    return { data: membersRef.current };
  },
  useQueryClient: () => ({
    setQueryData: vi.fn(),
    invalidateQueries: mockInvalidateQueries,
  }),
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "workspace-1",
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => workspaceRef.current,
}));

vi.mock("@multica/core/workspace/queries", () => ({
  memberListOptions: () => ({ queryKey: ["members"], queryFn: vi.fn() }),
  workspaceKeys: { list: () => ["workspaces"] },
}));

vi.mock("@multica/core/projects", () => ({
  workspaceResourcesOptions: (wsId: string) => ({
    queryKey: ["workspaces", wsId, "resources"],
    queryFn: vi.fn(),
  }),
  useCreateWorkspaceResource: () => ({ mutateAsync: mockCreateWorkspaceResource }),
  useDeleteWorkspaceResource: () => ({ mutateAsync: mockDeleteWorkspaceResource }),
}));

vi.mock("@multica/core/api", () => ({
  api: {
    updateWorkspace: mockUpdateWorkspace,
  },
}));

vi.mock("@multica/core/auth", () => {
  const useAuthStore = Object.assign(
    (sel?: (s: { user: { id: string } }) => unknown) =>
      sel ? sel({ user: { id: "user-1" } }) : { user: { id: "user-1" } },
    { getState: () => ({ user: { id: "user-1" } }) },
  );
  return { useAuthStore };
});

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

import { RepositoriesTab } from "./repositories-tab";

const TEST_RESOURCES = {
  en: { common: enCommon, settings: enSettings },
};

function I18nWrapper({ children }: { children: ReactNode }) {
  return (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      {children}
    </I18nProvider>
  );
}

const REPO_URL_PLACEHOLDER = "https://git.example.com/org/repo.git or git@git.example.com:org/repo.git";

describe("RepositoriesTab — view/edit toggle", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    workspaceRef.current = {
      id: "workspace-1",
      name: "Test Workspace",
      slug: "test-workspace",
      repos: [{ url: "https://github.com/multica-ai/multica" }],
    };
    membersRef.current = [{ user_id: "user-1", role: "owner" }];
    workspaceResourcesRef.current = [];
  });

  it("renders persisted repos in display mode (no input)", () => {
    render(<RepositoriesTab />, { wrapper: I18nWrapper });
    expect(screen.queryByDisplayValue("https://github.com/multica-ai/multica")).toBeNull();
    expect(screen.getByText("https://github.com/multica-ai/multica")).toBeTruthy();
  });

  it("Save button is disabled when clean", () => {
    render(<RepositoriesTab />, { wrapper: I18nWrapper });
    expect(screen.getByRole("button", { name: /^Save$/ })).toBeDisabled();
  });

  it("clicking Edit reveals an input pre-filled with the URL", async () => {
    const user = userEvent.setup();
    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: "Edit repository" }));

    expect(screen.getByDisplayValue("https://github.com/multica-ai/multica")).toBeTruthy();
  });

  it("Save re-enables after editing, then returns to display mode + disabled on success", async () => {
    const user = userEvent.setup();
    mockUpdateWorkspace.mockImplementation(async (_id: string, payload: { repos: { url: string; description?: string }[] }) => ({
      ...workspaceRef.current,
      repos: payload.repos,
    }));

    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: "Edit repository" }));
    const input = screen.getByDisplayValue("https://github.com/multica-ai/multica");
    await user.clear(input);
    await user.type(input, "https://github.com/multica-ai/edited");

    const saveBtn = screen.getByRole("button", { name: /^Save$/ });
    expect(saveBtn).not.toBeDisabled();

    // Simulate the workspace cache resync that the parent provider does
    // after a successful save — `setQueryData` updates the cache and the
    // useCurrentWorkspace hook would yield the new value on the next render.
    mockUpdateWorkspace.mockImplementationOnce(async (_id: string, payload: { repos: { url: string; description?: string }[] }) => {
      workspaceRef.current = { ...workspaceRef.current, repos: payload.repos };
      return workspaceRef.current;
    });

    await user.click(saveBtn);

    await waitFor(() => {
      expect(mockUpdateWorkspace).toHaveBeenCalled();
    });

    // After successful save, edit mode is cleared — input gone, Save disabled.
    await waitFor(() => {
      expect(screen.queryByDisplayValue("https://github.com/multica-ai/edited")).toBeNull();
    });
    expect(screen.getByRole("button", { name: /^Save$/ })).toBeDisabled();
  });

  it("newly added rows start in edit mode", async () => {
    const user = userEvent.setup();
    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    expect(screen.queryByPlaceholderText(REPO_URL_PLACEHOLDER)).toBeNull();
    await user.click(screen.getByRole("button", { name: /Add repository/ }));

    expect(screen.getByPlaceholderText(REPO_URL_PLACEHOLDER)).toBeTruthy();
    expect(screen.getByRole("button", { name: /^Save$/ })).not.toBeDisabled();
  });

  it("Edit clean row → Cancel returns to display mode without changing URL or dirtying Save", async () => {
    const user = userEvent.setup();
    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: "Edit repository" }));
    expect(screen.getByDisplayValue("https://github.com/multica-ai/multica")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Cancel edit" }));

    expect(screen.queryByDisplayValue("https://github.com/multica-ai/multica")).toBeNull();
    expect(screen.getByText("https://github.com/multica-ai/multica")).toBeTruthy();
    expect(screen.getByRole("button", { name: /^Save$/ })).toBeDisabled();
    expect(mockUpdateWorkspace).not.toHaveBeenCalled();
  });

  it("Cancel on a dirty edited row reverts the URL and exits edit mode", async () => {
    const user = userEvent.setup();
    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: "Edit repository" }));
    const input = screen.getByDisplayValue("https://github.com/multica-ai/multica") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "https://github.com/multica-ai/changed");
    expect(screen.getByRole("button", { name: /^Save$/ })).not.toBeDisabled();

    await user.click(screen.getByRole("button", { name: "Cancel edit" }));

    expect(screen.queryByDisplayValue("https://github.com/multica-ai/multica")).toBeNull();
    expect(screen.getByText("https://github.com/multica-ai/multica")).toBeTruthy();
    expect(screen.getByRole("button", { name: /^Save$/ })).toBeDisabled();
  });

  it("Cancel on a newly added (never saved) row removes the row entirely", async () => {
    const user = userEvent.setup();
    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: /Add repository/ }));
    expect(screen.getByPlaceholderText(REPO_URL_PLACEHOLDER)).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Cancel edit" }));

    expect(screen.queryByPlaceholderText(REPO_URL_PLACEHOLDER)).toBeNull();
    // Original persisted row is still there; the new empty row is gone.
    expect(screen.getByText("https://github.com/multica-ai/multica")).toBeTruthy();
    expect(screen.getByRole("button", { name: /^Save$/ })).toBeDisabled();
  });

  it("accepts scp-like shorthand without browser URL validation blocking submit", async () => {
    const user = userEvent.setup();
    mockUpdateWorkspace.mockImplementation(
      async (_id: string, payload: { repos: { url: string; description?: string }[] }) => {
        workspaceRef.current = { ...workspaceRef.current, repos: payload.repos };
        return workspaceRef.current;
      },
    );

    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    await user.click(screen.getByRole("button", { name: "Edit repository" }));
    const input = screen.getByDisplayValue("https://github.com/multica-ai/multica") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "git@github.com:multica-ai/multica.git");

    // type="text" (not "url") so the browser does not run native URL
    // validation; the value reaches the server which has the real check.
    expect(input.type).toBe("text");
    expect(input.validity.valid).toBe(true);

    await user.click(screen.getByRole("button", { name: /^Save$/ }));

    await waitFor(() => {
      expect(mockUpdateWorkspace).toHaveBeenCalledWith("workspace-1", {
        repos: [{ url: "git@github.com:multica-ai/multica.git" }],
      });
    });
  });

  it("deleting a row shifts tracked edit indices so the wrong row doesn't open", async () => {
    workspaceRef.current = {
      ...workspaceRef.current,
      repos: [{ url: "https://a.example/repo.git" }, { url: "https://b.example/repo.git" }],
    };
    const user = userEvent.setup();
    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    // Edit the second row.
    const editButtons = screen.getAllByRole("button", { name: "Edit repository" });
    await user.click(editButtons[1]!);
    expect(screen.getByDisplayValue("https://b.example/repo.git")).toBeTruthy();

    // Delete the first row. The remaining row should remain in edit mode
    // (its index dropped from 1 → 0).
    const deleteButtons = screen.getAllByRole("button", { name: "Delete repository" });
    await user.click(deleteButtons[0]!);

    const input = screen.getByDisplayValue("https://b.example/repo.git") as HTMLInputElement;
    expect(input.value).toBe("https://b.example/repo.git");
  });

  it("description field is editable and included in save payload", async () => {
    workspaceRef.current = {
      ...workspaceRef.current,
      repos: [{ url: "https://github.com/multica-ai/multica", description: "Main app" }],
    };
    const user = userEvent.setup();
    mockUpdateWorkspace.mockImplementation(
      async (_id: string, payload: { repos: { url: string; description?: string }[] }) => {
        workspaceRef.current = { ...workspaceRef.current, repos: payload.repos };
        return workspaceRef.current;
      },
    );

    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    // Description is shown in display mode.
    expect(screen.getByText("Main app")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Edit repository" }));
    const descriptionInput = screen.getByDisplayValue("Main app") as HTMLInputElement;

    await user.clear(descriptionInput);
    await user.type(descriptionInput, "Updated description");

    await user.click(screen.getByRole("button", { name: /^Save$/ }));

    await waitFor(() => {
      expect(mockUpdateWorkspace).toHaveBeenCalledWith("workspace-1", {
        repos: [{ url: "https://github.com/multica-ai/multica", description: "Updated description" }],
      });
    });
  });

  it("adds a workspace resource from the Resources page", async () => {
    mockCreateWorkspaceResource.mockResolvedValue({
      id: "resource-1",
      workspace_id: "workspace-1",
      resource_type: "feishu_drive",
      resource_ref: {
        drive_url: "https://feishu.cn/drive/folder",
        label: "Product docs",
      },
      label: "Product docs",
      created_at: "2026-06-26T00:00:00Z",
      updated_at: "2026-06-26T00:00:00Z",
      created_by: "user-1",
      can_manage: true,
    });

    const user = userEvent.setup();
    render(<RepositoriesTab />, { wrapper: I18nWrapper });

    fireEvent.change(screen.getByPlaceholderText("Drive URL or folder token"), {
      target: { value: "https://feishu.cn/drive/folder" },
    });
    fireEvent.change(screen.getAllByPlaceholderText("Optional name")[2]!, {
      target: { value: "Product docs" },
    });

    const addButton = screen.getAllByRole("button", { name: /^Add resource$/ })[2]!;
    await waitFor(() => {
      expect(addButton).not.toBeDisabled();
    });
    await user.click(addButton);

    await waitFor(() => {
      expect(mockCreateWorkspaceResource).toHaveBeenCalledWith({
        resource_type: "feishu_drive",
        resource_ref: {
          drive_url: "https://feishu.cn/drive/folder",
          label: "Product docs",
        },
        label: "Product docs",
      });
    });
  });
});
