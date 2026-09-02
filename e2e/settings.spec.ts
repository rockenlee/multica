import { test, expect, type Page } from "@playwright/test";
import { loginAsDefault, waitForPageText } from "./helpers";

async function waitForWorkspaceNameSave(page: Page) {
  return page.waitForResponse((response) => {
    if (response.request().method() !== "PATCH" || !response.ok()) return false;
    try {
      return /\/api\/workspaces\/[^/]+$/.test(new URL(response.url()).pathname);
    } catch {
      return false;
    }
  });
}

async function mockComposioEnabled(page: Page) {
  // Cloud keeps the product flag off. Self-host can turn it on, but the E2E
  // still forces it here so the connect flow does not depend on local env.
  await page.route("**/api/config", async (route) => {
    const response = await route.fetch();
    const body = (await response.json()) as {
      feature_flags?: Record<string, boolean>;
    };
    await route.fulfill({
      status: response.status(),
      contentType: "application/json",
      body: JSON.stringify({
        ...body,
        feature_flags: { ...body.feature_flags, composio_mcp_apps: true },
      }),
    });
  });
}

test.describe("Settings", () => {
  test("updating workspace name reflects in sidebar immediately", async ({
    page,
  }) => {
    const workspaceSlug = await loginAsDefault(page);

    await page.goto(`/${workspaceSlug}/settings?tab=workspace`, { waitUntil: "domcontentloaded" });
    await waitForPageText(page, "General");

    const nameInput = page.locator('input[name="workspace-name"]');
    await expect(nameInput).toBeVisible({ timeout: 10000 });
    const originalName = (await nameInput.inputValue()).trim();
    expect(originalName.length).toBeGreaterThan(0);

    const newName = "Renamed WS " + Date.now();
    const saved = waitForWorkspaceNameSave(page);
    await nameInput.fill(newName);
    await nameInput.press("Tab");
    await saved;

    await expect(page.getByText("Workspace settings saved").first()).toBeVisible({
      timeout: 10000,
    });
    await expect(page.getByRole("button", { name: newName }).first()).toBeVisible();

    const restored = waitForWorkspaceNameSave(page);
    await nameInput.fill(originalName);
    await nameInput.press("Tab");
    await restored;
    await expect(page.getByRole("button", { name: originalName }).first()).toBeVisible();
  });

  // Composio connect flow, fully mocked at the network boundary so it runs
  // without a configured COMPOSIO_API_KEY or a live Composio project. The
  // backend redirect is simulated by pointing the init endpoint's redirect_url
  // straight back at the settings page with ?connected=<slug> -- exercising the
  // frontend's callback toast + connections refresh (MUL-3718) end to end.
  test("connecting a Composio toolkit shows a toast and refreshes the list", async ({
    page,
  }) => {
    await mockComposioEnabled(page);

    const workspaceSlug = await loginAsDefault(page);
    const settingsUrl = `/${workspaceSlug}/settings?tab=integrations`;

    // Stateful: connections is empty until the (mocked) connect flow lands.
    let connected = false;

    await page.route("**/api/integrations/composio/toolkits", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify([
          { slug: "notion", name: "Notion", connectable: true },
        ]),
      }),
    );

    await page.route("**/api/integrations/composio/connections", (route) => {
      if (route.request().method() !== "GET") return route.fallback();
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify(
          connected
            ? [
                {
                  id: "conn-notion-1",
                  toolkit_slug: "notion",
                  status: "active",
                  connected_at: new Date().toISOString(),
                  last_used_at: null,
                },
              ]
            : [],
        ),
      });
    });

    await page.route("**/api/integrations/composio/connect/init", (route) => {
      // Composio would 302 through its hosted consent and back to our callback,
      // which emits CallbackRedirect's slug-less shape:
      // `/settings?tab=integrations&connected=<slug>`. The web proxy's
      // legacy-route redirect then prepends the last workspace slug, landing on
      // the real settings route. Mock that exact backend shape (NOT the final
      // slugged URL) so the test exercises the same redirect path real users hit.
      connected = true;
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          redirect_url: `/settings?tab=integrations&connected=notion`,
        }),
      });
    });

    await page.goto(settingsUrl, { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Notion").first()).toBeVisible({ timeout: 15000 });
    const connectButton = page.getByRole("button", { name: /^Connect$/ }).first();
    await expect(connectButton).toBeVisible({ timeout: 15000 });
    await connectButton.click();

    await expect(page.getByText("Connected").first()).toBeVisible({ timeout: 15000 });
    await expect(
      page.getByRole("button", { name: /Disconnect/ }).first(),
    ).toBeVisible({ timeout: 15000 });
    await expect(page).not.toHaveURL(/connected=notion/);
  });
});
