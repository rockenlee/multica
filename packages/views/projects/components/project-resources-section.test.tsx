import React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen, within } from "@testing-library/react";
import type { ProjectResource } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";

const mocks = vi.hoisted(() => ({
  resources: [] as ProjectResource[],
  createResource: vi.fn(),
  invalidateQueries: vi.fn(),
  updateResource: vi.fn(),
  deleteResource: vi.fn(),
}));

vi.mock("@tanstack/react-query", () => ({
  queryOptions: (options: unknown) => options,
  useQuery: (options: { queryKey?: readonly unknown[] }) => {
    const key = options.queryKey?.[0];
    if (key === "project-resources") {
      return { data: mocks.resources, isLoading: false };
    }
    if (key === "integrations") {
      // Enable every provider so the gated resource forms render.
      return {
        data: {
          connections: [
            { provider: "gitlab", sync_enabled: true },
            { provider: "feishu", sync_enabled: true },
            { provider: "zentao", sync_enabled: true },
          ],
        },
        isLoading: false,
      };
    }
    return { data: [], isLoading: false };
  },
  useQueryClient: () => ({
    invalidateQueries: mocks.invalidateQueries,
  }),
}));

vi.mock("@multica/core/projects", () => ({
  projectResourcesOptions: () => ({ queryKey: ["project-resources"] }),
  useCreateProjectResource: () => ({
    mutateAsync: mocks.createResource,
    isPending: false,
  }),
  useUpdateProjectResource: () => ({
    mutateAsync: mocks.updateResource,
    isPending: false,
  }),
  useDeleteProjectResource: () => ({
    mutateAsync: mocks.deleteResource,
    isPending: false,
  }),
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "workspace-1",
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({
    id: "workspace-1",
    name: "Test Workspace",
    slug: "test-workspace",
    repos: [],
  }),
}));

vi.mock("../../platform", () => ({
  isDesktopShell: () => false,
  pickDirectory: vi.fn(),
  useLocalDaemonStatus: () => ({
    daemonId: null,
    deviceName: null,
    running: false,
  }),
  validateLocalDirectory: vi.fn(),
}));

vi.mock("@multica/ui/components/ui/popover", () => ({
  Popover: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  PopoverTrigger: ({ render }: { render: React.ReactNode }) => <>{render}</>,
  PopoverContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

vi.mock("@multica/ui/components/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ render }: { render: React.ReactNode }) => <>{render}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <div role="tooltip">{children}</div>
  ),
}));

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

import { ProjectResourcesSection } from "./project-resources-section";

function renderSection() {
  return renderWithI18n(<ProjectResourcesSection projectId="project-1" />);
}

beforeEach(() => {
  mocks.resources = [];
  mocks.createResource.mockReset();
  mocks.invalidateQueries.mockReset();
  mocks.updateResource.mockReset();
  mocks.deleteResource.mockReset();
});

describe("ProjectResourcesSection enterprise resources", () => {
  it("can attach a Feishu Drive resource from the project Resources panel", async () => {
    mocks.createResource.mockResolvedValue({
      id: "resource-1",
      project_id: "project-1",
      workspace_id: "workspace-1",
      resource_type: "feishu_drive",
      resource_ref: {
        drive_url: "https://feishu.cn/drive/folder",
        label: "Product docs",
      },
      label: "Product docs",
      position: 0,
      created_at: "2026-06-26T00:00:00Z",
      created_by: "user-1",
    });

    renderSection();
    fireEvent.click(screen.getByRole("button", { name: "Add resource" }));

    const form = screen.getByText("Feishu Drive").closest("form");
    if (!form) throw new Error("Feishu Drive form not found");
    fireEvent.change(
      within(form).getByPlaceholderText("Drive folder URL, folder token, or node token"),
      { target: { value: "https://feishu.cn/drive/folder" } },
    );
    fireEvent.change(within(form).getByPlaceholderText("Display name (optional)"), {
      target: { value: "Product docs" },
    });
    fireEvent.click(within(form).getByRole("button", { name: "Add" }));

    expect(mocks.createResource).toHaveBeenCalledWith({
      resource_type: "feishu_drive",
      resource_ref: {
        drive_url: "https://feishu.cn/drive/folder",
        label: "Product docs",
      },
      label: "Product docs",
    });
  });

  it("does not render sync binding controls in the project Resources panel", () => {
    renderSection();

    expect(screen.queryByText("Sync bindings")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save sync binding" })).not.toBeInTheDocument();
  });
});
